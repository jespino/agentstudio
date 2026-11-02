package agentctx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestWriteKeepsUserContent(t *testing.T) {
	dir := t.TempDir()
	c := Context{
		ID:        "demo",
		Product:   "Demo Studio",
		Agents:    Expand([]byte("Edit `{{FILE}}`.\n"), map[string]string{"FILE": "talk.demo"}),
		SkillName: "demo",
		Skill:     map[string][]byte{"SKILL.md": []byte("# Skill\n"), "reference.md": []byte("# Ref\n")},
		Files:     map[string][]byte{".demo/docs/DSL.md": []byte("# DSL\n"), ".demo/empty": nil},
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# My notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for range 2 { // idempotent
		if err := Write(dir, c); err != nil {
			t.Fatal(err)
		}
	}
	begin, end := c.Markers()
	if begin != "<!-- demo:begin (managed by Demo Studio; edits inside this block are overwritten) -->" || end != "<!-- demo:end -->" {
		t.Fatalf("markers: %q %q", begin, end)
	}
	agents, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	s := string(agents)
	if !strings.HasPrefix(s, "# My notes\n") || strings.Count(s, begin) != 1 || !strings.Contains(s, "Edit `talk.demo`.") {
		t.Fatalf("AGENTS.md:\n%s", s)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md")); string(b) != "mine\n" {
		t.Fatalf("CLAUDE.md overwritten: %q", b)
	}
	if _, err := os.Stat(filepath.Join(dir, "GEMINI.md")); !os.IsNotExist(err) {
		t.Fatal("GEMINI.md was written")
	}
	for p, want := range map[string]string{
		".demo/docs/DSL.md":                "# DSL\n",
		".claude/skills/demo/SKILL.md":     "# Skill\n",
		".agents/skills/demo/SKILL.md":     "# Skill\n",
		".agents/skills/demo/reference.md": "# Ref\n",
	} {
		if b, _ := os.ReadFile(filepath.Join(dir, p)); string(b) != want {
			t.Fatalf("%s: %q", p, b)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".demo", "empty")); !os.IsNotExist(err) {
		t.Fatal("an empty file was written")
	}

	// Editing the block updates it in place.
	c.Agents = []byte("New text.\n")
	if err := Write(dir, c); err != nil {
		t.Fatal(err)
	}
	agents, _ = os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if s := string(agents); strings.Count(s, begin) != 1 || strings.Contains(s, "talk.demo") || !strings.Contains(s, "New text.") || !strings.HasPrefix(s, "# My notes\n") {
		t.Fatalf("AGENTS.md after an update:\n%s", s)
	}
}

func TestWriteCreatesImports(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, Context{ID: "demo", Product: "Demo Studio", Agents: []byte("x\n")}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md")); string(b) != "@AGENTS.md\n" {
		t.Fatalf("CLAUDE.md: %q", b)
	}
}

func TestSkillFS(t *testing.T) {
	fsys := fstest.MapFS{
		"skills/x/SKILL.md": {Data: []byte("a")},
		"skills/x/more.md":  {Data: []byte("b")},
		"skills/x/img.png":  {Data: []byte("c")},
	}
	got, err := SkillFS(fsys, "skills/x/*.md")
	if err != nil || len(got) != 2 || string(got["SKILL.md"]) != "a" || string(got["more.md"]) != "b" {
		t.Fatalf("%v %v", got, err)
	}
	if _, err := SkillFS(fsys, "nope/*.md"); err == nil {
		t.Fatal("expected an error for no files")
	}
}
