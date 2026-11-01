package studio

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jespino/agentstudio/rpc"
	"github.com/jespino/agentstudio/terminal"
)

// events records what a studio emits.
type events struct {
	mu   sync.Mutex
	list []event
}

type event struct {
	name string
	data any
}

func (e *events) emit(name string, data any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.list = append(e.list, event{name, data})
}

// wait returns the first event called name after the n-th one.
func (e *events) wait(t *testing.T, name string, from int) (event, int) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		e.mu.Lock()
		for i := from; i < len(e.list); i++ {
			if e.list[i].name == name {
				ev := e.list[i]
				e.mu.Unlock()
				return ev, i + 1
			}
		}
		e.mu.Unlock()
	}
	t.Fatalf("no %s event", name)
	return event{}, 0
}

func newWorkspace(t *testing.T, opts WorkspaceOptions) (*Workspace, *events) {
	t.Helper()
	cfg := Config{
		ID: "demo", Ext: ".demo", Default: "project.demo",
		RecentFile: filepath.Join(t.TempDir(), "recent.json"),
		Agent: Agent{
			Context: func(dir, file string) error {
				return os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(filepath.Base(file)), 0o644)
			},
			Env: func(dir, file string) []string { return []string{"DEMO_PROJECT=" + dir, "DEMO_FILE=" + file} },
		},
	}
	w := NewWorkspace(cfg, opts)
	ev := &events{}
	w.SetEmitter(ev.emit)
	t.Cleanup(w.Shutdown)
	return w, ev
}

func TestWorkspaceLifecycle(t *testing.T) {
	changed := make(chan string, 8)
	w, ev := newWorkspace(t, WorkspaceOptions{
		Template: func(name string) string { return "demo " + name + "\n" },
		Prepare:  func(dir string) error { return os.MkdirAll(filepath.Join(dir, "assets"), 0o755) },
		Settings: func(dir string) any {
			b, _ := os.ReadFile(filepath.Join(dir, "demo.json"))
			return string(b)
		},
		OnChange: func(path string) { changed <- filepath.Base(path) },
	})
	if w.CurrentProject() != nil {
		t.Fatal("a project is open")
	}
	dir := filepath.Join(t.TempDir(), "My Talk")
	p, err := w.NewProject(dir, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Path != filepath.Join(dir, "My Talk.demo") || p.Name != "My Talk" || p.Source != "demo My Talk\n" {
		t.Fatalf("project %+v", p)
	}
	if st, err := os.Stat(filepath.Join(dir, "assets")); err != nil || !st.IsDir() {
		t.Fatal("Prepare did not run")
	}
	if _, err := w.NewProject(dir, "", ""); err == nil {
		t.Fatal("creating over an existing project should fail")
	}
	_, n := ev.wait(t, EventProject, 0)

	// The studio saves; that's not echoed back. An edit on disk is.
	if err := w.SaveSource("mine\n"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.Path, []byte("agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// os.WriteFile truncates first: the watcher may see the empty file
	// before the content.
	for {
		var e event
		e, n = ev.wait(t, EventSource, n)
		ch := e.data.(SourceChange)
		if ch.Path != p.Path || (ch.Source != "" && ch.Source != "agent\n") {
			t.Fatalf("source change %+v", ch)
		}
		if ch.Source == "agent\n" {
			break
		}
	}
	if got := w.CurrentProject(); got.Source != "agent\n" {
		t.Fatalf("current source %q", got.Source)
	}

	// Other files: the hook, and Refresh reloads the settings.
	os.WriteFile(filepath.Join(dir, "demo.json"), []byte("{}"), 0o644)
	select {
	case name := <-changed:
		if name != "demo.json" {
			t.Fatalf("OnChange %q", name)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("OnChange not called")
	}
	if p := w.Refresh(); p.Settings != "{}" {
		t.Fatalf("settings %v", p.Settings)
	}
	ev.wait(t, EventProject, n)

	// Opening by folder; the recent list.
	if p2, err := w.OpenProject(dir); err != nil || p2.Path != p.Path {
		t.Fatalf("open folder: %v %v", p2, err)
	}
	if r := w.RecentProjects(); len(r) != 1 || r[0].Path != p.Path || r[0].Name != "My Talk" || !r[0].Exists {
		t.Fatalf("recent %+v", r)
	}
	w.CloseProject()
	if w.CurrentProject() != nil {
		t.Fatal("still open")
	}
	if d, _ := w.Project(); d != "" {
		t.Fatal("Base still has the project")
	}
}

func TestResolve(t *testing.T) {
	b := New(Config{ID: "demo", Ext: ".demo", Default: "project.demo"})
	dir := t.TempDir()
	write := func(name string) string {
		p := filepath.Join(dir, name)
		os.WriteFile(p, nil, 0o644)
		return p
	}
	if _, err := b.Resolve(dir); err == nil || !strings.Contains(err.Error(), "has no .demo file") {
		t.Fatalf("empty folder: %v", err)
	}
	one := write("one.demo")
	if got, err := b.Resolve(dir); err != nil || got != one {
		t.Fatalf("one file: %v %v", got, err)
	}
	write("two.demo")
	if _, err := b.Resolve(dir); err == nil || !strings.Contains(err.Error(), "several") {
		t.Fatalf("two files: %v", err)
	}
	def := write("project.demo")
	if got, err := b.Resolve(dir); err != nil || got != def {
		t.Fatalf("default: %v %v", got, err)
	}
	if _, err := b.Resolve(write("notes.txt")); err == nil {
		t.Fatal("a .txt file should not open")
	}
	f := New(Config{ID: "demo", Ext: ".demo", FolderProjects: true})
	if got, err := f.Resolve(dir); err != nil || got != dir {
		t.Fatalf("folder project: %v %v", got, err)
	}
	if _, err := f.Resolve(one); err == nil {
		t.Fatal("folder projects: a file should not open")
	}
}

func TestRecentNamesAndLegacyEntries(t *testing.T) {
	file := filepath.Join(t.TempDir(), "recent.json")
	b := New(Config{ID: "demo", Ext: ".demo", Default: "project.demo", RecentFile: file})
	for path, want := range map[string]string{
		"/x/talk/project.demo": "talk",
		"/x/talk/talk.demo":    "talk",
		"/x/talk/intro.demo":   "talk / intro",
	} {
		if got := b.recentName(path); got != want {
			t.Errorf("%s: %q, want %q", path, got, want)
		}
	}
	// Folder lists saved as {"dir": …} (TCAD Studio) still load.
	dir := t.TempDir()
	os.WriteFile(file, []byte(`[{"dir": "`+filepath.ToSlash(dir)+`", "name": "old"}]`), 0o644)
	if r := b.RecentProjects(); len(r) != 1 || r[0].Path != filepath.ToSlash(dir) || r[0].Name != "old" {
		t.Fatalf("legacy entries: %+v", r)
	}
}

func TestListDir(t *testing.T) {
	b := New(Config{ID: "demo", Ext: ".demo", Browse: func(name string) bool { return strings.HasSuffix(name, ".mp4") }})
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "proj"), 0o755)
	os.WriteFile(filepath.Join(dir, "proj", "a.demo"), nil, 0o644)
	os.MkdirAll(filepath.Join(dir, "plain"), 0o755)
	os.MkdirAll(filepath.Join(dir, ".hidden"), 0o755)
	for _, f := range []string{"b.demo", "clip.mp4", "notes.txt"} {
		os.WriteFile(filepath.Join(dir, f), nil, 0o644)
	}
	l, err := b.ListDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range l.Entries {
		s := e.Name
		switch {
		case e.IsProject:
			s += "/project"
		case e.IsDir:
			s += "/"
		case e.IsScript:
			s += ":script"
		case e.IsOther:
			s += ":other"
		}
		got = append(got, s)
	}
	want := []string{"plain/", "proj/project", "b.demo:script", "clip.mp4:other"}
	if !slices.Equal(got, want) || !l.IsProject || !slices.Equal(l.Scripts, []string{"b.demo"}) {
		t.Fatalf("listing %v (project %v, scripts %v), want %v", got, l.IsProject, l.Scripts, want)
	}
}

func TestTerminalLaunchModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses shell scripts as fake agent CLIs")
	}
	for _, tc := range []struct {
		harness string
		resume  bool
		args    string
	}{
		{"claude", false, ""},
		{"claude", true, "--resume"},
		{"codex", true, "resume"},
		{"gemini", true, "--resume"},
	} {
		t.Run(tc.harness+"/"+tc.args, func(t *testing.T) {
			w, ev := newWorkspace(t, WorkspaceOptions{})
			dir := t.TempDir()
			p, err := w.NewProject(dir, "talk", "x")
			if err != nil {
				t.Fatal(err)
			}
			// The fake agent records how it was run, without contacting an
			// agent service or relying on a local session history.
			bin := t.TempDir()
			script := "#!/bin/sh\nprintf '%s\\n' \"$*\" \"$PWD\" \"$DEMO_PROJECT\" \"$DEMO_FILE\" > launch.txt\n"
			if err := os.WriteFile(filepath.Join(bin, tc.harness), []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin)
			launch := w.StartTerminal
			if tc.resume {
				launch = w.ResumeTerminal
			}
			id, err := launch(tc.harness, 80, 24)
			if err != nil {
				t.Fatal(err)
			}
			start, n := ev.wait(t, EventTerminalStart, 0)
			if s := start.data.(map[string]string); s["id"] != id || s["harness"] != tc.harness {
				t.Fatalf("start event %v", s)
			}
			exit, _ := ev.wait(t, terminal.EventExit, n)
			if e := exit.data.(map[string]any); e["id"] != id || e["code"] != 0 {
				t.Fatalf("unexpected exit: %v", e)
			}
			got, err := os.ReadFile(filepath.Join(dir, "launch.txt"))
			want := strings.Join([]string{tc.args, dir, dir, p.Path, ""}, "\n")
			if err != nil || string(got) != want {
				t.Fatalf("launch: %q, %v; want %q", got, err, want)
			}
			if b, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md")); string(b) != "talk.demo" {
				t.Fatalf("agent context: %q", b)
			}
		})
	}
}

func TestTerminalErrors(t *testing.T) {
	w, _ := newWorkspace(t, WorkspaceOptions{})
	if _, err := w.StartTerminal("shell", 80, 24); err == nil {
		t.Fatal("an agent started without a project")
	}
	if _, err := w.NewProject(t.TempDir(), "", "x"); err != nil {
		t.Fatal(err)
	}
	for _, harness := range []string{"shell", "agy", "unknown"} {
		if _, err := w.ResumeTerminal(harness, 80, 24); err == nil {
			t.Fatalf("expected an error resuming %s", harness)
		}
	}
	if sessions := w.Terminals(); len(sessions) != 0 {
		t.Fatalf("unsupported resume started sessions: %v", sessions)
	}
}

func TestToolOnPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	exe := filepath.Join(t.TempDir(), "real-demo")
	os.WriteFile(exe, []byte("#!/bin/sh\necho \"demo tool $1\" > \"$DEMO_PROJECT/tool.txt\"\n"), 0o755)
	w := NewWorkspace(Config{
		ID: "demo", Ext: ".demo", RecentFile: filepath.Join(t.TempDir(), "r.json"),
		Agent: Agent{Tool: "demo", ToolExe: exe, Env: func(dir, file string) []string { return []string{"DEMO_PROJECT=" + dir} }},
	}, WorkspaceOptions{})
	ev := &events{}
	w.SetEmitter(ev.emit)
	defer w.Shutdown()
	dir := t.TempDir()
	if _, err := w.NewProject(dir, "", "x"); err != nil {
		t.Fatal(err)
	}
	id, err := w.StartTerminal("shell", 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	w.TerminalWrite(id, "demo check; exit\n")
	ev.wait(t, terminal.EventExit, 0)
	if b, _ := os.ReadFile(filepath.Join(dir, "tool.txt")); string(b) != "demo tool check\n" {
		t.Fatalf("tool output %q", b)
	}
}

// app is a studio's service, as the studios define it.
type app struct {
	*Workspace
}

func (a *app) Extra(x string) string { return x }
func (a *app) Setup(x string)        {}
func (a *app) HiddenMethods() []string {
	return append(a.Workspace.HiddenMethods(), "Setup")
}

func TestRPCSurface(t *testing.T) {
	a := &app{NewWorkspace(Config{ID: "demo", Ext: ".demo"}, WorkspaceOptions{})}
	m := rpc.Methods(a)
	for _, name := range []string{"Extra", "OpenProject", "NewProject", "SaveSource", "CurrentProject", "CloseProject",
		"RecentProjects", "ListDir", "HasNativeDialogs", "ChooseDirectory", "Harnesses", "StartTerminal", "ResumeTerminal",
		"TerminalWrite", "TerminalResize", "TerminalBuffer", "StopTerminal", "Terminals"} {
		if _, ok := m[name]; !ok {
			t.Errorf("%s should be an RPC", name)
		}
	}
	for _, name := range []string{"Setup", "Shutdown", "SetEmitter", "SetDialogs", "Emit", "Dialogs", "SetProject",
		"Project", "Resolve", "Config", "Current", "Create", "Refresh", "HiddenMethods"} {
		if _, ok := m[name]; ok {
			t.Errorf("%s should not be an RPC", name)
		}
	}
}

// saveDialog answers the save dialog with the next path of answers.
type saveDialog struct {
	answers []string
	starts  *[]string
}

func (saveDialog) ChooseDirectory(string, string, bool) (string, error)   { return "", nil }
func (saveDialog) ChooseFiles(string, string, []Filter) ([]string, error) { return nil, nil }
func (d *saveDialog) SaveFile(title, start, name string) (string, error) {
	*d.starts = append(*d.starts, start+"|"+name)
	a := d.answers[0]
	d.answers = d.answers[1:]
	return a, nil
}

func TestSaveAs(t *testing.T) {
	w, _ := newWorkspace(t, WorkspaceOptions{})
	if _, err := w.SaveAs("Save", "", "x.pdf", nil); err == nil {
		t.Fatal("saved without native dialogs")
	}
	proj := t.TempDir()
	if _, err := w.NewProject(proj, "", "x"); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	var starts []string
	w.SetDialogs(&saveDialog{answers: []string{filepath.Join(out, "trip"), "", filepath.Join(out, "b.pdf")}, starts: &starts})
	// The dialog starts in the project; the suggested extension is added.
	if p, err := w.SaveAs("Export", "", "trip.pdf", []byte("pdf")); err != nil || p != filepath.Join(out, "trip.pdf") {
		t.Fatalf("first save: %q %v", p, err)
	}
	if b, _ := os.ReadFile(filepath.Join(out, "trip.pdf")); string(b) != "pdf" {
		t.Fatalf("saved %q", b)
	}
	// Cancelled: nothing written, no error.
	if p, err := w.SaveAs("Export", "", "c.pdf", []byte("x")); p != "" || err != nil {
		t.Fatalf("cancel: %q %v", p, err)
	}
	// Then where the last file went.
	if _, err := w.SaveAs("Export", "/elsewhere", "b.pdf", []byte("b")); err != nil {
		t.Fatal(err)
	}
	if want := []string{proj + "|trip.pdf", out + "|c.pdf", out + "|b.pdf"}; !slices.Equal(starts, want) {
		t.Fatalf("dialog starts %v, want %v", starts, want)
	}
}
