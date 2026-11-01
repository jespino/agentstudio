// Package studio is the backend shared by agent studios: desktop apps with a
// preview of a text document (a DSL) on one side and a coding agent working
// on it in a terminal on the other.
//
// Base holds what every studio has: native dialogs, the recent projects, a
// folder browser for shells without dialogs, and the agent terminal (with
// the context files agents read written before they start). Workspace adds
// the usual project: a folder holding one source file, edited by the studio
// and the agent alike, with every change on disk pushed to the UI.
//
// A studio's service struct embeds one of them, so their methods are part of
// its API, both for Wails (desktop) and for package rpc (browser).
package studio

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jespino/agentstudio/terminal"
)

// EventTerminalStart is sent when an agent starts, so every open page shows
// it. Payload: {id, harness}.
const EventTerminalStart = "terminal:start"

// Emitter delivers events to the frontend.
type Emitter func(name string, data any)

// Dialogs are native dialogs, provided by the desktop shell. In a browser
// there are none and the UI uses its own folder browser (ListDir).
type Dialogs interface {
	// ChooseDirectory returns "" if the user cancels.
	ChooseDirectory(title, start string, canCreate bool) (string, error)
	// ChooseFiles returns nil if the user cancels.
	ChooseFiles(title, start string, filters []Filter) ([]string, error)
	// SaveFile asks where to save a file, starting in start with name
	// suggested. It returns "" if the user cancels.
	SaveFile(title, start, name string) (string, error)
}

// Filter is a file type choice of ChooseFiles: Pattern is "*.mp4;*.mov".
type Filter struct {
	Name    string
	Pattern string
}

// Config describes a studio's projects.
type Config struct {
	// ID is a short name: the folder in the user's config directory where the
	// recent projects are kept (<config>/<ID>/recent.json).
	ID string
	// Ext is the extension of the project files (".kino").
	Ext string
	// Default is the file opened in a folder holding several project files
	// ("project.kino"); its stem also names recent projects by their folder.
	Default string
	// FolderProjects makes a project a folder rather than a file: any folder
	// opens, and those holding Ext files are marked as projects.
	FolderProjects bool
	// Browse reports other files the folder browser lists (as IsOther).
	Browse func(name string) bool
	// Agent configures the agents run in the terminal.
	Agent Agent
	// RecentFile overrides where recent projects are kept (tests).
	RecentFile string
}

// Agent configures the agents run in the terminal panel.
type Agent struct {
	// Context writes the files agents read (AGENTS.md, the skill, …) into the
	// project folder before an agent starts (see package agentctx). file is
	// the project file ("" for folder projects).
	Context func(dir, file string) error
	// Env adds environment variables for the agent.
	Env func(dir, file string) []string
	// Tool is a command the agent runs to check its work ("tcad"). The
	// studio puts it first on the agent's PATH, so it needs no separate
	// install. "" adds nothing.
	Tool string
	// ToolExe is the program run as Tool; "" is the running program. A path
	// that does not exist is skipped (the Tool on the PATH is used).
	ToolExe string
}

// Base is the part of the backend every studio shares. Embed a *Base (or a
// *Workspace) in the service struct.
type Base struct {
	cfg   Config
	terms *terminal.Manager

	mu      sync.Mutex
	emit    Emitter
	dialogs Dialogs
	dir     string // the open project folder ("" when none)
	file    string // the open project file ("" for folder projects)
	toolDir string // holds the Tool link put first on the agent's PATH
	tooled  bool
	saveDir string // where SaveAs saved last
}

// New creates the shared backend.
func New(cfg Config) *Base {
	b := &Base{cfg: cfg, emit: func(string, any) {}}
	b.terms = terminal.NewManager(b.Emit)
	return b
}

// HiddenMethods are the exported methods that are not part of the API (see
// rpc.Hider). A service adding its own includes these.
func (b *Base) HiddenMethods() []string {
	return []string{"Emit", "Dialogs", "SetProject", "Project", "Resolve", "Config", "SetEmitter", "SetDialogs", "SaveAs", "StudioHTTP"}
}

// Config returns the configuration.
func (b *Base) Config() Config { return b.cfg }

// SetEmitter installs the event sink (the shell creates it after the service).
func (b *Base) SetEmitter(emit Emitter) {
	if emit == nil {
		emit = func(string, any) {}
	}
	b.mu.Lock()
	b.emit = emit
	b.mu.Unlock()
}

// Emit sends an event to the frontend.
func (b *Base) Emit(name string, data any) {
	b.mu.Lock()
	emit := b.emit
	b.mu.Unlock()
	emit(name, data)
}

// SetDialogs installs native dialogs.
func (b *Base) SetDialogs(d Dialogs) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.dialogs = d
}

// Dialogs returns the native dialogs, or nil in a browser.
func (b *Base) Dialogs() Dialogs {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.dialogs
}

// Shutdown stops the agents.
func (b *Base) Shutdown() {
	b.terms.StopAll()
	b.mu.Lock()
	dir := b.toolDir
	b.toolDir, b.tooled = "", false
	b.mu.Unlock()
	if dir != "" {
		os.RemoveAll(dir)
	}
}

// SetProject records the open project (dir "" when it closes) and adds it
// to the recent ones. Agents started for another project are stopped: they
// must not keep editing it from a panel that now belongs to this one.
func (b *Base) SetProject(dir, file string) {
	b.mu.Lock()
	switching := b.dir != dir || b.file != file
	b.dir, b.file = dir, file
	b.mu.Unlock()
	if switching {
		b.terms.StopAll()
	}
	if dir == "" {
		return
	}
	if file != "" {
		b.remember(file)
	} else {
		b.remember(dir)
	}
}

// Project returns the open project's folder and file ("" when none).
func (b *Base) Project() (dir, file string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.dir, b.file
}

// ---- Project files -------------------------------------------------------------

// ExpandHome expands a leading ~ to the home folder.
func ExpandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

func (b *Base) isProjectFile(name string) bool {
	return b.cfg.Ext != "" && strings.EqualFold(filepath.Ext(name), b.cfg.Ext) && !strings.HasPrefix(name, ".")
}

// scripts lists the project files directly inside dir.
func (b *Base) scripts(dir string) []string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if !e.IsDir() && b.isProjectFile(e.Name()) {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// Resolve turns a path the user gave (a project file or folder) into the
// absolute path to open, with an error a user understands. A folder must
// hold the Default file, or exactly one project file. With FolderProjects,
// it is the folder.
func (b *Base) Resolve(path string) (string, error) {
	path = ExpandHome(path)
	st, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("%s does not exist", path)
	}
	if b.cfg.FolderProjects {
		if !st.IsDir() {
			return "", fmt.Errorf("%s is not a folder", path)
		}
		return filepath.Abs(path)
	}
	if !st.IsDir() {
		if !b.isProjectFile(filepath.Base(path)) {
			return "", fmt.Errorf("%s is not a %s file", filepath.Base(path), b.cfg.Ext)
		}
		return filepath.Abs(path)
	}
	found := b.scripts(path)
	if b.cfg.Default != "" && containsFold(found, b.cfg.Default) {
		return filepath.Abs(filepath.Join(path, b.cfg.Default))
	}
	switch len(found) {
	case 1:
		return filepath.Abs(filepath.Join(path, found[0]))
	case 0:
		return "", fmt.Errorf("%s has no %s file; create a new project there instead", path, b.cfg.Ext)
	}
	return "", fmt.Errorf("%s has several %s files (%s); open one of them", path, b.cfg.Ext, strings.Join(found, ", "))
}

func containsFold(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}

// WriteAtomic writes through a temporary file, so watchers never see half a
// file. An existing file keeps its permissions.
func WriteAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if st, err := os.Stat(path); err == nil {
		os.Chmod(tmp.Name(), st.Mode().Perm())
	} else {
		os.Chmod(tmp.Name(), 0o644)
	}
	return os.Rename(tmp.Name(), path)
}

// ---- Recent projects -------------------------------------------------------

const maxRecent = 12

// RecentProject is an entry of the recent list.
type RecentProject struct {
	Path   string    `json:"path"` // the project file (the folder for folder projects)
	Name   string    `json:"name"`
	Opened time.Time `json:"opened"`
	Exists bool      `json:"exists"`
}

// UnmarshalJSON also reads entries saved as {"dir": …}.
func (r *RecentProject) UnmarshalJSON(b []byte) error {
	type plain RecentProject
	var v struct {
		plain
		Dir string `json:"dir"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*r = RecentProject(v.plain)
	if r.Path == "" {
		r.Path = v.Dir
	}
	return nil
}

func (b *Base) recentFile() string {
	if b.cfg.RecentFile != "" {
		return b.cfg.RecentFile
	}
	dir, err := os.UserConfigDir()
	if err != nil || b.cfg.ID == "" {
		return ""
	}
	return filepath.Join(dir, b.cfg.ID, "recent.json")
}

func (b *Base) loadRecent() []RecentProject {
	var list []RecentProject
	if f := b.recentFile(); f != "" {
		if data, err := os.ReadFile(f); err == nil {
			json.Unmarshal(data, &list)
		}
	}
	return list
}

func (b *Base) recentName(path string) string {
	if b.cfg.FolderProjects {
		return filepath.Base(path)
	}
	stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	dir := filepath.Base(filepath.Dir(path))
	if stem == dir || (b.cfg.Default != "" && strings.EqualFold(stem, strings.TrimSuffix(b.cfg.Default, filepath.Ext(b.cfg.Default)))) {
		return dir
	}
	return dir + " / " + stem
}

func (b *Base) remember(path string) {
	f := b.recentFile()
	if f == "" {
		return
	}
	list := []RecentProject{{Path: path, Name: b.recentName(path), Opened: time.Now()}}
	for _, r := range b.loadRecent() {
		if r.Path != path && len(list) < maxRecent {
			list = append(list, r)
		}
	}
	os.MkdirAll(filepath.Dir(f), 0o755)
	if data, err := json.MarshalIndent(list, "", "  "); err == nil {
		os.WriteFile(f, data, 0o644)
	}
}

// RecentProjects lists recently opened projects, newest first.
func (b *Base) RecentProjects() []RecentProject {
	list := b.loadRecent()
	for i := range list {
		_, err := os.Stat(list[i].Path)
		list[i].Exists = err == nil
	}
	if list == nil {
		list = []RecentProject{}
	}
	return list
}

// ---- Folder browser (for shells without native dialogs) --------------------

// DirEntry is an entry of a folder listing.
type DirEntry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	IsDir     bool   `json:"isDir"`
	IsProject bool   `json:"isProject"` // a folder holding project files
	IsScript  bool   `json:"isScript"`  // a project file
	IsOther   bool   `json:"isOther"`   // another file Config.Browse lists
}

// DirListing is the content of a folder.
type DirListing struct {
	Path      string     `json:"path"`
	Parent    string     `json:"parent,omitempty"`
	IsProject bool       `json:"isProject"` // the folder holds project files
	Scripts   []string   `json:"scripts"`   // its project files
	Entries   []DirEntry `json:"entries"`
}

// ListDir lists a folder for the in-app browser ("" = home). Hidden entries
// are skipped and folders come first; the only files listed are project
// files and those Config.Browse accepts.
func (b *Base) ListDir(path string) (*DirListing, error) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		path = home
	}
	path, err := filepath.Abs(ExpandHome(path))
	if err != nil {
		return nil, err
	}
	ents, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	out := &DirListing{Path: path, Scripts: b.scripts(path), Entries: []DirEntry{}}
	if out.Scripts == nil {
		out.Scripts = []string{}
	}
	out.IsProject = len(out.Scripts) > 0
	if parent := filepath.Dir(path); parent != path {
		out.Parent = parent
	}
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		full := filepath.Join(path, e.Name())
		isDir := e.IsDir()
		if e.Type()&os.ModeSymlink != 0 {
			if st, err := os.Stat(full); err == nil {
				isDir = st.IsDir()
			}
		}
		de := DirEntry{Name: e.Name(), Path: full, IsDir: isDir}
		switch {
		case isDir:
			de.IsProject = len(b.scripts(full)) > 0
		case b.isProjectFile(e.Name()):
			de.IsScript = true
		case b.cfg.Browse != nil && b.cfg.Browse(e.Name()):
			de.IsOther = true
		default:
			continue
		}
		out.Entries = append(out.Entries, de)
	}
	sort.Slice(out.Entries, func(i, j int) bool {
		a, c := out.Entries[i], out.Entries[j]
		if a.IsDir != c.IsDir {
			return a.IsDir
		}
		return strings.ToLower(a.Name) < strings.ToLower(c.Name)
	})
	return out, nil
}

// ---- Native dialogs ----------------------------------------------------------

// HasNativeDialogs reports whether the shell has native dialogs.
func (b *Base) HasNativeDialogs() bool { return b.Dialogs() != nil }

// ChooseDirectory shows a native folder picker ("" if cancelled), starting
// next to the open project.
func (b *Base) ChooseDirectory(title string, canCreate bool) (string, error) {
	d := b.Dialogs()
	if d == nil {
		return "", errors.New("no native dialogs in this shell")
	}
	start := ""
	if dir, _ := b.Project(); dir != "" {
		start = filepath.Dir(dir)
	} else if home, err := os.UserHomeDir(); err == nil {
		start = home
	}
	return d.ChooseDirectory(title, start, canCreate)
}

// ---- Agent terminal ----------------------------------------------------------

// Harnesses lists the agent CLIs (and a shell) the terminal panel can run.
func (b *Base) Harnesses() []terminal.Harness { return terminal.DefaultHarnesses() }

// StartTerminal runs a harness in the project folder, after writing the
// context files agents read.
func (b *Base) StartTerminal(harness string, cols, rows int) (string, error) {
	return b.startTerminal(harness, cols, rows, false)
}

// ResumeTerminal runs a harness so that it continues one of its saved
// conversations in the project folder instead of starting a new one.
func (b *Base) ResumeTerminal(harness string, cols, rows int) (string, error) {
	return b.startTerminal(harness, cols, rows, true)
}

func (b *Base) startTerminal(harness string, cols, rows int, resume bool) (string, error) {
	dir, file := b.Project()
	if dir == "" {
		return "", errors.New("no project is open")
	}
	var h *terminal.Harness
	for _, x := range terminal.DefaultHarnesses() {
		if x.ID == harness {
			h = &x
			break
		}
	}
	if h == nil {
		return "", fmt.Errorf("unknown agent %q", harness)
	}
	if resume {
		r, err := h.ForResume()
		if err != nil {
			return "", err
		}
		h = &r
	}
	// Refresh the context every time: the user may have edited or deleted it.
	if h.ID != "shell" && b.cfg.Agent.Context != nil {
		if err := b.cfg.Agent.Context(dir, file); err != nil {
			return "", fmt.Errorf("writing the agent context: %w", err)
		}
	}
	var env []string
	if b.cfg.Agent.Env != nil {
		env = b.cfg.Agent.Env(dir, file)
	}
	if bin := b.tool(); bin != "" {
		env = append(env, "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	id, err := b.terms.Start(*h, dir, env, cols, rows)
	if err == nil {
		// Other pages (or the one that asked, before it gets the reply)
		// attach to it if they show no agent.
		b.Emit(EventTerminalStart, map[string]string{"id": id, "harness": h.ID})
	}
	return id, err
}

// tool makes (once) a folder holding a link named Agent.Tool to its program,
// for the agent's PATH.
func (b *Base) tool() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.tooled || b.cfg.Agent.Tool == "" {
		return b.toolDir
	}
	b.tooled = true
	exe := b.cfg.Agent.ToolExe
	if exe == "" {
		var err error
		if exe, err = os.Executable(); err != nil {
			return ""
		}
	} else if _, err := os.Stat(exe); err != nil {
		return ""
	}
	dir, err := os.MkdirTemp("", b.cfg.ID+"-agent-")
	if err != nil {
		return ""
	}
	name := b.cfg.Agent.Tool
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	link := filepath.Join(dir, name)
	if os.Symlink(exe, link) != nil && os.Link(exe, link) != nil {
		os.RemoveAll(dir)
		return ""
	}
	b.toolDir = dir
	return dir
}

func (b *Base) TerminalWrite(id, data string) error            { return b.terms.Write(id, data) }
func (b *Base) TerminalResize(id string, cols, rows int) error { return b.terms.Resize(id, cols, rows) }
func (b *Base) TerminalBuffer(id string) (string, error)       { return b.terms.Buffer(id) }
func (b *Base) StopTerminal(id string) error                   { return b.terms.Stop(id) }

// Terminals lists the terminal sessions, oldest first.
func (b *Base) Terminals() []terminal.Info {
	list := b.terms.List()
	sort.Slice(list, func(i, j int) bool {
		return len(list[i].ID) < len(list[j].ID) || (len(list[i].ID) == len(list[j].ID) && list[i].ID < list[j].ID)
	})
	return list
}
