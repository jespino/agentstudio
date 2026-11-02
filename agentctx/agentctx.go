// Package agentctx writes the context files that coding agents (Claude Code,
// Codex, OpenCode, Antigravity, …) read when they start in a project
// folder, so an agent launched from a studio's terminal panel knows the
// studio's language and how to check its work:
//
//	AGENTS.md                  the studio's managed block; whatever the user
//	                           writes outside it is kept
//	CLAUDE.md                  "@AGENTS.md", only if missing
//	.agents/skills/<name>/     the skill, where Codex, OpenCode and
//	                           Antigravity look for it
//	.claude/skills/<name>/     the same skill again, for Claude Code
//	any other Files            generated (a language reference, …)
package agentctx

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Context is what a studio writes into a project folder.
type Context struct {
	// ID names the managed block of AGENTS.md ("kino" → "<!-- kino:begin …").
	// It must not change between versions: an AGENTS.md written with another
	// ID would get a second block.
	ID string
	// Product is the studio's name, as shown in the block's begin marker.
	Product string
	// Agents is the content of the managed block of AGENTS.md.
	Agents []byte
	// Imports are files created with "@AGENTS.md" when missing, for agents
	// that read their own file. Nil means CLAUDE.md.
	Imports []string
	// Skill is the agent skill: its files (SKILL.md, references, …) by name,
	// written to .agents/skills/<SkillName>/ and .claude/skills/<SkillName>/.
	SkillName string
	Skill     map[string][]byte
	// Files are other generated files, by slash-separated path relative to
	// the project folder. Empty contents are skipped.
	Files map[string][]byte
}

// skillRoots are the folders agents look for skills in: .agents/skills is the
// shared one (Codex, OpenCode, Antigravity); Claude Code only reads
// .claude/skills. Each gets a real copy, not a link, so the folder works on
// any system and survives being copied around.
var skillRoots = []string{".agents", ".claude"}

// Markers returns the lines around the managed block of AGENTS.md.
func (c Context) Markers() (begin, end string) {
	return fmt.Sprintf("<!-- %s:begin (managed by %s; edits inside this block are overwritten) -->", c.ID, c.Product),
		fmt.Sprintf("<!-- %s:end -->", c.ID)
}

// Write creates or refreshes the context files in dir. Files whose content
// is unchanged are not touched, so watchers don't see spurious edits.
func Write(dir string, c Context) error {
	if c.ID == "" {
		return fmt.Errorf("agentctx: the context has no ID")
	}
	begin, end := c.Markers()
	if err := writeManaged(filepath.Join(dir, "AGENTS.md"), c.Agents, begin, end); err != nil {
		return err
	}
	imports := c.Imports
	if imports == nil {
		imports = []string{"CLAUDE.md"}
	}
	for _, name := range imports {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			if err := os.WriteFile(p, []byte("@AGENTS.md\n"), 0o644); err != nil {
				return err
			}
		}
	}
	for name, content := range c.Skill {
		for _, root := range skillRoots {
			if err := writeIfChanged(filepath.Join(dir, root, "skills", c.SkillName, filepath.FromSlash(name)), content); err != nil {
				return err
			}
		}
	}
	for name, content := range c.Files {
		if len(content) == 0 {
			continue
		}
		if err := writeIfChanged(filepath.Join(dir, filepath.FromSlash(name)), content); err != nil {
			return err
		}
	}
	return nil
}

// SkillFS reads a skill from the files matching pattern in fsys (for
// instance "skills/kino/*.md"), keyed by base name, for Context.Skill.
func SkillFS(fsys fs.FS, pattern string) (map[string][]byte, error) {
	files, err := fs.Glob(fsys, pattern)
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	for _, f := range files {
		b, err := fs.ReadFile(fsys, f)
		if err != nil {
			return nil, err
		}
		out[path.Base(f)] = b
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("agentctx: no skill files match %s", pattern)
	}
	return out, nil
}

// Expand replaces {{KEY}} in tmpl with vars[KEY].
func Expand(tmpl []byte, vars map[string]string) []byte {
	for k, v := range vars {
		tmpl = bytes.ReplaceAll(tmpl, []byte("{{"+k+"}}"), []byte(v))
	}
	return tmpl
}

func writeIfChanged(p string, content []byte) error {
	if old, err := os.ReadFile(p); err == nil && bytes.Equal(old, content) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, content, 0o644)
}

// writeManaged replaces the managed block of a file, keeping everything the
// user wrote outside it.
func writeManaged(p string, block []byte, begin, end string) error {
	managed := begin + "\n" + strings.TrimRight(string(block), "\n") + "\n" + end + "\n"
	old, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return os.WriteFile(p, []byte(managed), 0o644)
	}
	if err != nil {
		return err
	}
	s := string(old)
	i := strings.Index(s, begin)
	j := strings.Index(s, end)
	var out string
	if i >= 0 && j > i {
		out = s[:i] + managed + strings.TrimPrefix(s[j+len(end):], "\n")
	} else {
		out = strings.TrimRight(s, "\n") + "\n\n" + managed
	}
	if out == s {
		return nil
	}
	return os.WriteFile(p, []byte(out), 0o644)
}
