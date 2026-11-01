package studio

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// Events a Workspace sends to the frontend.
const (
	EventProject = "project:changed" // payload: *Project (nil when closed)
	EventSource  = "project:source"  // payload: SourceChange (the file changed on disk)
)

// Project is the open project of a Workspace.
type Project struct {
	Path   string `json:"path"` // the project file
	Dir    string `json:"dir"`  // the project folder
	Name   string `json:"name"` // the folder's name
	Source string `json:"source"`
	// Settings are the studio's per-project settings (WorkspaceOptions.Settings).
	Settings any `json:"settings,omitempty"`
}

// SourceChange is the project file's new content after an edit on disk.
type SourceChange struct {
	Path   string `json:"path"`
	Source string `json:"source"`
}

// WorkspaceOptions are the hooks of a Workspace.
type WorkspaceOptions struct {
	// Template returns the source of a new project when none is given; name
	// is the project folder's name.
	Template func(name string) string
	// Prepare readies the folder of a new project before its file is written
	// (an assets/ folder, …).
	Prepare func(dir string) error
	// Settings loads the per-project settings shown in Project.Settings.
	Settings func(dir string) any
	// OnChange is told about changes to other files of the project folder
	// (a settings file, …). The source file's changes are handled already.
	OnChange func(path string)
}

// Workspace is a Base whose projects are a folder holding one source file.
// The studio's editor saves it (SaveSource), an agent edits it too, and
// edits made on disk reach the UI as EventSource.
type Workspace struct {
	*Base
	opts WorkspaceOptions

	mu        sync.Mutex
	project   *Project
	disk      []byte // the file as last read or written by us
	stopWatch func()
}

// NewWorkspace creates the backend of a studio with one-file projects.
func NewWorkspace(cfg Config, opts WorkspaceOptions) *Workspace {
	return &Workspace{Base: New(cfg), opts: opts}
}

// HiddenMethods are the exported methods that are not part of the API.
func (w *Workspace) HiddenMethods() []string {
	return append(w.Base.HiddenMethods(), "Current", "Create", "Refresh")
}

// Shutdown stops the agents and the file watching.
func (w *Workspace) Shutdown() {
	w.Base.Shutdown()
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopWatch != nil {
		w.stopWatch()
		w.stopWatch = nil
	}
}

// Current returns a copy of the open project (its Source as opened).
func (w *Workspace) Current() (*Project, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.project == nil {
		return nil, errors.New("no project is open")
	}
	p := *w.project
	return &p, nil
}

// CurrentProject returns the open project with its current source, or nil.
func (w *Workspace) CurrentProject() *Project {
	p, err := w.Current()
	if err != nil {
		return nil
	}
	if b, err := os.ReadFile(p.Path); err == nil {
		p.Source = string(b)
	}
	return p
}

// OpenProject opens a project file, or a folder holding one.
func (w *Workspace) OpenProject(path string) (*Project, error) {
	file, err := w.Resolve(path)
	if err != nil {
		return nil, err
	}
	src, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(file)
	p := &Project{Path: file, Dir: dir, Name: filepath.Base(dir), Source: string(src)}
	if w.opts.Settings != nil {
		p.Settings = w.opts.Settings(dir)
	}
	stop, err := w.watch(file)
	if err != nil {
		return nil, err
	}
	w.mu.Lock()
	if w.stopWatch != nil {
		w.stopWatch()
	}
	w.project, w.disk, w.stopWatch = p, src, stop
	w.mu.Unlock()
	w.SetProject(dir, file)
	w.Emit(EventProject, p)
	return p, nil
}

// CloseProject goes back to the welcome screen.
func (w *Workspace) CloseProject() {
	w.mu.Lock()
	if w.stopWatch != nil {
		w.stopWatch()
	}
	w.project, w.disk, w.stopWatch = nil, nil, nil
	w.mu.Unlock()
	w.SetProject("", "")
	w.Emit(EventProject, nil)
}

// NewProject creates dir (if needed) with <name><Ext> holding source (the
// Template when empty), and opens it. name defaults to the folder's name.
// It fails if the file exists already.
func (w *Workspace) NewProject(dir, name, source string) (*Project, error) {
	return w.Create(dir, name, source, nil)
}

// Create is NewProject with an extra step: prepare fills the new folder
// (after WorkspaceOptions.Prepare) before the file is written.
func (w *Workspace) Create(dir, name, source string, prepare func(dir string) error) (*Project, error) {
	if dir == "" {
		return nil, errors.New("choose a folder for the project")
	}
	dir, err := filepath.Abs(ExpandHome(dir))
	if err != nil {
		return nil, err
	}
	if name == "" {
		name = filepath.Base(dir)
	}
	name = filepath.Base(name)
	if !strings.EqualFold(filepath.Ext(name), w.cfg.Ext) {
		name += w.cfg.Ext
	}
	file := filepath.Join(dir, name)
	if _, err := os.Stat(file); err == nil {
		return nil, fmt.Errorf("%s already exists; open it instead", file)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	for _, fn := range []func(string) error{w.opts.Prepare, prepare} {
		if fn != nil {
			if err := fn(dir); err != nil {
				return nil, err
			}
		}
	}
	if source == "" && w.opts.Template != nil {
		source = w.opts.Template(filepath.Base(dir))
	}
	if err := os.WriteFile(file, []byte(source), 0o644); err != nil {
		return nil, err
	}
	return w.OpenProject(file)
}

// SaveSource writes the project file (the studio's editor).
func (w *Workspace) SaveSource(source string) error {
	p, err := w.Current()
	if err != nil {
		return err
	}
	b := []byte(source)
	w.mu.Lock()
	w.disk = b
	w.mu.Unlock()
	return WriteAtomic(p.Path, b)
}

// Refresh reloads the project's settings and sends the project again.
func (w *Workspace) Refresh() *Project {
	w.mu.Lock()
	if w.project == nil {
		w.mu.Unlock()
		return nil
	}
	if w.opts.Settings != nil {
		w.project.Settings = w.opts.Settings(w.project.Dir)
	}
	w.mu.Unlock()
	p := w.CurrentProject()
	w.Emit(EventProject, p)
	return p
}

// watch reports edits of file made by others (an agent, another editor).
// Editors often replace files by renaming, so the folder is watched.
func (w *Workspace) watch(file string) (func(), error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := fw.Add(filepath.Dir(file)); err != nil {
		fw.Close()
		return nil, err
	}
	go func() {
		for {
			select {
			case ev, ok := <-fw.Events:
				if !ok {
					return
				}
				if !ev.Has(fsnotify.Write | fsnotify.Create | fsnotify.Rename) {
					continue
				}
				name := filepath.Clean(ev.Name)
				if name == file {
					w.reload(file)
				} else if strings.HasPrefix(filepath.Base(name), "."+filepath.Base(file)+".") {
					continue // WriteAtomic's temporary file
				} else if w.opts.OnChange != nil {
					w.opts.OnChange(name)
				}
			case _, ok := <-fw.Errors:
				if !ok {
					return
				}
			}
		}
	}()
	return func() { fw.Close() }, nil
}

func (w *Workspace) reload(file string) {
	b, err := os.ReadFile(file)
	if err != nil {
		return // mid-rename: the Create event that follows reloads it
	}
	w.mu.Lock()
	if w.project == nil || w.project.Path != file || bytes.Equal(b, w.disk) {
		w.mu.Unlock()
		return
	}
	w.disk = b
	w.mu.Unlock()
	w.Emit(EventSource, SourceChange{Path: file, Source: string(b)})
}
