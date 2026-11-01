package studio

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
)

// StudioRoute is the path prefix of Base's own endpoints. The desktop and
// browser shells serve it (StudioHTTP) next to the service's API.
const StudioRoute = "/__studio/"

// SaveAs asks where to save a file the studio produced (an export, a mesh)
// with the native save dialog, and writes data there. The dialog starts in
// the folder of the last file saved, else start, else the project folder.
// It returns where the file went, "" if the user cancelled.
func (b *Base) SaveAs(title, start, name string, data []byte) (string, error) {
	d := b.Dialogs()
	if d == nil {
		return "", errors.New("no native dialogs in this shell")
	}
	b.mu.Lock()
	if b.saveDir != "" {
		start = b.saveDir
	} else if start == "" {
		start = b.dir
	}
	b.mu.Unlock()
	if title == "" {
		title = "Save"
	}
	path, err := d.SaveFile(title, start, filepath.Base(name))
	if err != nil || path == "" {
		return "", err
	}
	// Dialogs that don't add the extension of the suggested name
	if ext := filepath.Ext(name); ext != "" && !strings.EqualFold(filepath.Ext(path), ext) {
		path += ext
	}
	if err := WriteAtomic(path, data); err != nil {
		return "", err
	}
	b.mu.Lock()
	b.saveDir = filepath.Dir(path)
	b.mu.Unlock()
	return path, nil
}

// maxSave bounds what the UI can post to save (a rendered video).
const maxSave = 4 << 30

// StudioHTTP serves Base's endpoints, under StudioRoute:
//
//	POST /__studio/save?name=<file>&title=<dialog title>
//	     the body is the file: SaveAs, then {"saved": bool, "path": "…"}
//
// The desktop webviews ignore <a download>, so the UI saves through here
// (saveFile in the frontend package does it, or downloads in a browser).
func (b *Base) StudioHTTP() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reply := func(code int, v any) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
			json.NewEncoder(w).Encode(v)
		}
		switch r.URL.Path {
		case StudioRoute + "save":
			if r.Method != http.MethodPost {
				reply(http.StatusMethodNotAllowed, map[string]string{"error": "use POST"})
				return
			}
			if b.Dialogs() == nil {
				reply(http.StatusNotImplemented, map[string]string{"error": "no native dialogs in this shell"})
				return
			}
			data, err := io.ReadAll(io.LimitReader(r.Body, maxSave))
			if err != nil {
				reply(http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			q := r.URL.Query()
			name := q.Get("name")
			if name == "" {
				name = "untitled"
			}
			path, err := b.SaveAs(q.Get("title"), "", name, data)
			if err != nil {
				reply(http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			reply(http.StatusOK, map[string]any{"saved": path != "", "path": path})
		default:
			http.NotFound(w, r)
		}
	})
}
