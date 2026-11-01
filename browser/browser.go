// Package browser serves a studio's UI for a browser: its frontend, its
// service over package rpc, and its own endpoints. It needs no cgo or
// WebKit, so command-line programs can offer it ("app serve") without the
// desktop shell (package desktop, which uses it for "serve").
package browser

import (
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/jespino/agentstudio/rpc"
	"github.com/jespino/agentstudio/studio"
)

// Service is a studio's backend: a pointer to a struct embedding a
// *studio.Base (or *studio.Workspace).
type Service interface {
	SetEmitter(studio.Emitter)
	Shutdown()
}

// studioHandler is a service embedding a studio.Base: its own endpoints
// (saving files, …) are served under studio.StudioRoute.
type studioHandler interface {
	StudioHTTP() http.Handler
}

// Options describe what is served.
type Options struct {
	// ID is the program's short name (it prefixes the session cookie).
	ID string
	// Name is the app's name, for messages.
	Name string
	// Assets is the built frontend (index.html at its root); nil serves the
	// API only (a development server like Vite has the frontend).
	Assets fs.FS
	// Routes are path prefixes ("/__kino/") the service, an http.Handler,
	// serves itself, with the path unchanged.
	Routes []string
	// Open opens a project given on the command line.
	Open func(path string) error
	// Addr is the default listen address of Serve.
	Addr string
}

// Listen serves the UI on addr and returns the server and the URL to open
// (it carries the session token: AGENTSTUDIO_TOKEN, if set, so a development
// proxy can send it too). The service's events go to the server's pages.
func Listen(svc Service, o Options, addr string) (*rpc.Server, string, error) {
	routes := map[string]http.Handler{}
	if len(o.Routes) > 0 {
		h, ok := svc.(http.Handler)
		if !ok {
			return nil, "", fmt.Errorf("browser: Routes are set but %T is not an http.Handler", svc)
		}
		for _, p := range o.Routes {
			routes[p] = h
		}
	}
	if sh, ok := svc.(studioHandler); ok {
		routes[studio.StudioRoute] = sh.StudioHTTP()
	}
	var static http.Handler
	if o.Assets != nil {
		static = http.FileServerFS(o.Assets)
	}
	srv := rpc.New(svc, rpc.Options{Name: o.ID, Token: os.Getenv(rpc.TokenEnv), Static: static, Routes: routes})
	svc.SetEmitter(srv.Broadcast)
	url, err := srv.Listen(addr)
	if err != nil {
		return nil, "", err
	}
	return srv, url, nil
}

// Serve runs the UI for a browser until interrupted. args: [-addr host:port]
// [project].
func Serve(svc Service, o Options, args []string) {
	name := o.ID
	if name == "" {
		name = filepath.Base(os.Args[0])
	}
	flags := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := flags.String("addr", o.Addr, "listen address (keep it on loopback)")
	flags.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s serve [-addr host:port] [project]\n", name)
		flags.PrintDefaults()
	}
	flags.Parse(args)
	if *addr == "" {
		*addr = "127.0.0.1:0"
	}
	srv, url, err := Listen(svc, o, *addr)
	if err != nil {
		log.Fatal(err)
	}
	defer svc.Shutdown()
	defer srv.Close()
	if p := flags.Arg(0); p != "" && o.Open != nil {
		if err := o.Open(p); err != nil {
			log.Fatal(err)
		}
	}
	log.Printf("%s at %s", o.Name, url)
	Wait()
}

// Wait blocks until the program is interrupted (Ctrl-C, SIGTERM).
func Wait() {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
}
