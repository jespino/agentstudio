// Package desktop runs a studio: as a desktop app (Wails 3, a native window
// showing the embedded frontend) or, with "serve", the same UI for a
// browser (package browser, which needs no cgo: use it directly in programs
// that must build without WebKit).
//
//	app [project]                        the desktop app
//	app serve [-addr host:port] [project]  the UI in a browser
package desktop

import (
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/jespino/agentstudio/browser"
	"github.com/jespino/agentstudio/studio"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// Service is a studio's backend: a pointer to a struct embedding a
// *studio.Base (or *studio.Workspace).
type Service interface {
	SetEmitter(studio.Emitter)
	SetDialogs(studio.Dialogs)
	Shutdown()
}

// RGB is a window background color.
type RGB struct{ R, G, B uint8 }

// Options describe a studio's app.
type Options struct {
	// ID is the program's short name (the session cookie of "serve").
	ID          string
	Name        string // the app's name ("Kino Studio")
	Description string
	Title       string // the window's title; Name by default
	Width       int    // 1600 by default
	Height      int    // 960 by default
	Background  *RGB   // the window's color while the UI loads

	// Assets is the built frontend (index.html at its root).
	Assets fs.FS
	// Routes are path prefixes ("/__kino/") the service, an http.Handler,
	// serves itself, with the path unchanged.
	Routes []string
	// Open opens a project given on the command line ("" if none).
	Open func(path string) error

	// Setup adds native integration to the desktop app (more windows, audio
	// devices, …); the cleanup it returns runs when the app ends.
	Setup func(wa *application.App) (cleanup func())

	// ServeAddr is the default address of "serve".
	ServeAddr string
}

// Main runs the app as the command line asks: "serve" or the desktop app.
func Main[T any, S interface {
	*T
	Service
}](svc S, o Options) {
	if len(os.Args) > 1 && os.Args[1] == "serve" {
		browser.Serve(svc, o.Browser(), os.Args[2:])
		return
	}
	Run(svc, o, os.Args[1:])
}

func routes(svc Service, o Options, next http.Handler) http.Handler {
	if len(o.Routes) == 0 {
		return next
	}
	h, ok := svc.(http.Handler)
	if !ok {
		log.Fatalf("desktop: Routes are set but %T is not an http.Handler", svc)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, p := range o.Routes {
			if strings.HasPrefix(r.URL.Path, p) {
				h.ServeHTTP(w, r)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// Run runs the desktop app; args may name a project to open.
func Run[T any, S interface {
	*T
	Service
}](svc S, o Options, args []string) {
	if o.Title == "" {
		o.Title = o.Name
	}
	if o.Width == 0 {
		o.Width, o.Height = 1600, 960
	}
	wa := application.New(application.Options{
		Name:        o.Name,
		Description: o.Description,
		Services:    []application.Service{application.NewService(svc)},
		Assets: application.AssetOptions{
			Handler:    application.AssetFileServerFS(o.Assets),
			Middleware: func(next http.Handler) http.Handler { return routes(svc, o, next) },
		},
		OnShutdown: svc.Shutdown,
		Mac:        application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
	})
	svc.SetEmitter(func(name string, data any) { wa.Event.Emit(name, data) })
	svc.SetDialogs(Dialogs{wa})
	if o.Setup != nil {
		if cleanup := o.Setup(wa); cleanup != nil {
			defer cleanup()
		}
	}

	win := application.WebviewWindowOptions{Title: o.Title, Width: o.Width, Height: o.Height, URL: "/"}
	if o.Background != nil {
		win.BackgroundColour = application.NewRGB(o.Background.R, o.Background.G, o.Background.B)
	}
	wa.Window.NewWithOptions(win)

	// A project given on the command line opens directly; otherwise the UI
	// shows the welcome screen (new / open / recent).
	if len(args) > 0 && o.Open != nil {
		wa.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
			if err := o.Open(args[0]); err != nil {
				wa.Dialog.Error().SetTitle("Cannot open " + args[0]).SetMessage(err.Error()).Show()
			}
		})
	}
	if err := wa.Run(); err != nil {
		log.Fatal(err)
	}
}

// Browser returns the options of the UI in a browser ("serve").
func (o Options) Browser() browser.Options {
	return browser.Options{ID: o.ID, Name: o.Name, Assets: o.Assets, Routes: o.Routes, Open: o.Open, Addr: o.ServeAddr}
}

// Dialogs adapts Wails' native dialogs to studio.Dialogs.
type Dialogs struct{ App *application.App }

// ChooseDirectory shows a folder picker.
func (d Dialogs) ChooseDirectory(title, start string, canCreate bool) (string, error) {
	dlg := d.App.Dialog.OpenFile().
		SetTitle(title).
		CanChooseDirectories(true).
		CanChooseFiles(false).
		CanCreateDirectories(canCreate)
	if start != "" {
		dlg.SetDirectory(start)
	}
	if w := d.App.Window.Current(); w != nil {
		dlg.AttachToWindow(w)
	}
	return dlg.PromptForSingleSelection()
}

// ChooseFiles shows a picker for several files.
func (d Dialogs) ChooseFiles(title, start string, filters []studio.Filter) ([]string, error) {
	dlg := d.App.Dialog.OpenFile().
		SetTitle(title).
		CanChooseFiles(true).
		CanChooseDirectories(false)
	for _, f := range filters {
		dlg.AddFilter(f.Name, f.Pattern)
	}
	if start != "" {
		dlg.SetDirectory(start)
	}
	if w := d.App.Window.Current(); w != nil {
		dlg.AttachToWindow(w)
	}
	return dlg.PromptForMultipleSelection()
}

// SaveFile asks where to save a file.
func (d Dialogs) SaveFile(title, start, name string) (string, error) {
	dlg := d.App.Dialog.SaveFile().SetFilename(name).CanCreateDirectories(true)
	if title != "" {
		dlg.SetMessage(title)
	}
	if ext := filepath.Ext(name); ext != "" {
		dlg.AddFilter(ext[1:]+" files", "*"+ext)
	}
	if start != "" {
		dlg.SetDirectory(start)
	}
	if w := d.App.Window.Current(); w != nil {
		dlg.AttachToWindow(w)
	}
	return dlg.PromptForSingleSelection()
}
