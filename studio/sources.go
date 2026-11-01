package studio

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The project's source files, for the UI's source panel when a project has
// several (folder projects): list them, read one, save one. Paths are
// slash-separated and relative to the project folder.

// SourceFiles lists the project files (Config.Ext) of the open project:
// those in its folder, and in its subfolders for folder projects (hidden
// folders and node_modules are skipped).
func (b *Base) SourceFiles() ([]string, error) {
	dir, _ := b.Project()
	if dir == "" {
		return nil, errors.New("no project is open")
	}
	out := []string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entries are left out
		}
		if d.IsDir() {
			if p != dir && (!b.cfg.FolderProjects || strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if b.isProjectFile(d.Name()) {
			rel, _ := filepath.Rel(dir, p)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}

// sourcePath resolves rel to a project file of the open project.
func (b *Base) sourcePath(rel string) (string, error) {
	dir, _ := b.Project()
	if dir == "" {
		return "", errors.New("no project is open")
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if rel == "" || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%q is not a file of the project", rel)
	}
	if !b.isProjectFile(filepath.Base(clean)) {
		return "", fmt.Errorf("%s is not a %s file", rel, b.cfg.Ext)
	}
	return filepath.Join(dir, clean), nil
}

// ReadSource returns the content of a project file.
func (b *Base) ReadSource(rel string) (string, error) {
	p, err := b.sourcePath(rel)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	b.rememberSource(p, string(data))
	return string(data), nil
}

// WriteSource saves a project file (the source panel). The change is not
// sent back as EventSource.
func (b *Base) WriteSource(rel, text string) error {
	p, err := b.sourcePath(rel)
	if err != nil {
		return err
	}
	if err := WriteAtomic(p, []byte(text)); err != nil {
		return err
	}
	b.rememberSource(p, text)
	return nil
}

func (b *Base) rememberSource(path, text string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.sources == nil {
		b.sources = map[string]string{}
	}
	b.sources[path] = text
}

// SourceChanged tells the UI that a project file may have changed on disk
// (a studio's watcher calls it): if its content is not what the UI last read
// or wrote, EventSource carries the new one ({path, source}, the path
// absolute like a Workspace's).
func (b *Base) SourceChanged(path string) {
	dir, _ := b.Project()
	if dir == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	b.mu.Lock()
	old, known := b.sources[path]
	if known && old == string(data) {
		b.mu.Unlock()
		return
	}
	if b.sources == nil {
		b.sources = map[string]string{}
	}
	b.sources[path] = string(data)
	b.mu.Unlock()
	if !known {
		return // the UI never read it: nothing to update
	}
	b.Emit(EventSource, SourceChange{Path: path, Source: string(data)})
}
