// Package rpc exposes a studio's service over plain HTTP, mirroring what the
// Wails runtime provides in the desktop shell:
//
//	POST /rpc/<Method>  JSON array of arguments → {"result": …} | {"error": "…"}
//	GET  /events        server-sent events: {"name": …, "data": …}
//	     <route>…       the service's own endpoints (Options.Routes)
//	     /              the frontend (Options.Static)
//
// It makes the UI usable in a regular browser, for development and tests.
// The service can start programs (the agent terminal), so the server only
// answers its own pages:
//
//   - every request needs the session token: the start URL (?token=…) sets
//     it as a cookie, and a development proxy can send it in a header;
//   - the Host header must be one of the server's own addresses, so a page
//     elsewhere can't reach it by pointing one of its names at 127.0.0.1
//     (DNS rebinding);
//   - a call authenticated by the cookie must come from the server's own
//     origin, and be JSON (which a cross-site form can't send).
package rpc

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"
)

// TokenHeader carries the session token on requests from a development
// proxy (Vite), which can't hold the cookie.
const TokenHeader = "X-Agentstudio-Token"

// TokenEnv names the environment variable a development setup uses to share
// the token between the server and its proxy.
const TokenEnv = "AGENTSTUDIO_TOKEN"

// hiddenByDefault are methods every service has that are not RPCs.
var hiddenByDefault = []string{"ServeHTTP", "Shutdown", "Close", "ServiceStartup", "ServiceShutdown", "ServiceName", "HiddenMethods"}

// Hider is implemented by services with exported methods that are not RPCs
// (setup, internal plumbing). Methods with interface, func or channel
// parameters are never RPCs anyway.
type Hider interface {
	HiddenMethods() []string
}

// Options configures a Server.
type Options struct {
	// Name prefixes the session cookie ("kino" → "kino_session_34116").
	Name string
	// Token every request must carry; "" generates one. NoToken disables
	// the check (tests).
	Token   string
	NoToken bool
	// Static serves the frontend (may be nil).
	Static http.Handler
	// Routes are the service's own endpoints by path prefix ("/__kino/"),
	// served with the path unchanged.
	Routes map[string]http.Handler
}

// Server is an http.Handler.
type Server struct {
	opts    Options
	service reflect.Value
	methods map[string]reflect.Value
	cookie  string

	mu    sync.Mutex
	hosts []string // accepted Host headers; nil accepts any (before Listen)
	subs  map[chan []byte]struct{}
	done  chan struct{}
	stop  sync.Once
	ln    net.Listener
}

// New exposes service's exported methods. Wire its events to the stream
// with Broadcast (service.SetEmitter(srv.Broadcast)).
func New(service any, opts Options) *Server {
	if opts.Token == "" && !opts.NoToken {
		b := make([]byte, 24)
		rand.Read(b)
		opts.Token = hex.EncodeToString(b)
	}
	if opts.Name == "" {
		opts.Name = "agentstudio"
	}
	s := &Server{
		opts:    opts,
		service: reflect.ValueOf(service),
		methods: Methods(service),
		cookie:  opts.Name + "_session",
		subs:    map[chan []byte]struct{}{},
		done:    make(chan struct{}),
	}
	return s
}

// Token returns the session token.
func (s *Server) Token() string { return s.opts.Token }

// Methods returns the methods of service callable over RPC, by name.
func Methods(service any) map[string]reflect.Value {
	v := reflect.ValueOf(service)
	hidden := slices.Clone(hiddenByDefault)
	if h, ok := service.(Hider); ok {
		hidden = append(hidden, h.HiddenMethods()...)
	}
	out := map[string]reflect.Value{}
	t := v.Type()
outer:
	for i := range t.NumMethod() {
		m := t.Method(i)
		if !m.IsExported() || slices.Contains(hidden, m.Name) {
			continue
		}
		mt := m.Type
		for j := 1; j < mt.NumIn(); j++ { // 0 is the receiver
			if j == 1 && mt.In(j) == ctxType {
				continue // the server passes the request's context
			}
			switch mt.In(j).Kind() {
			case reflect.Interface, reflect.Func, reflect.Chan, reflect.UnsafePointer:
				continue outer
			}
		}
		if mt.IsVariadic() {
			continue
		}
		out[m.Name] = v.Method(i)
	}
	return out
}

// Listen serves on addr (a loopback address) and returns the URL to open,
// which carries the session token. Only the listen address is accepted as
// Host from then on.
func (s *Server) Listen(addr string) (string, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", fmt.Errorf("cannot listen on %s: %w", addr, err)
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	s.mu.Lock()
	s.ln = ln
	s.hosts = []string{ln.Addr().String(), "localhost:" + port, "127.0.0.1:" + port}
	// Cookies don't keep ports apart: two servers on one host need two names.
	s.cookie = s.opts.Name + "_session_" + port
	s.mu.Unlock()
	go (&http.Server{Handler: s, ReadHeaderTimeout: 10 * time.Second}).Serve(ln)
	u := "http://" + ln.Addr().String() + "/"
	if s.opts.Token != "" {
		u += "?token=" + url.QueryEscape(s.opts.Token)
	}
	return u, nil
}

// Close stops listening and ends the event streams.
func (s *Server) Close() {
	s.stop.Do(func() {
		close(s.done)
		s.mu.Lock()
		if s.ln != nil {
			s.ln.Close()
		}
		s.mu.Unlock()
	})
}

func (s *Server) validToken(t string) bool {
	return t != "" && subtle.ConstantTimeCompare([]byte(t), []byte(s.opts.Token)) == 1
}

// ServeHTTP checks the request and serves it.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	hosts, cookie := s.hosts, s.cookie
	s.mu.Unlock()
	if hosts != nil && !slices.Contains(hosts, r.Host) {
		http.Error(w, "unexpected host", http.StatusForbidden)
		return
	}
	if !s.opts.NoToken {
		if t := r.URL.Query().Get("token"); t != "" && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
			if !s.validToken(t) {
				http.Error(w, "invalid token", http.StatusForbidden)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: cookie, Value: s.opts.Token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
			q := r.URL.Query()
			q.Del("token")
			u := *r.URL
			u.RawQuery = q.Encode()
			http.Redirect(w, r, u.RequestURI(), http.StatusFound)
			return
		}
		byHeader := s.validToken(r.Header.Get(TokenHeader))
		if !byHeader {
			c, err := r.Cookie(cookie)
			if err != nil || !s.validToken(c.Value) {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprintln(w, "Open the full address (with ?token=…) the server printed when it started.")
				return
			}
			// The cookie goes with any request the browser makes, even one
			// another site triggers: only accept our own pages' writes.
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				if o := r.Header.Get("Origin"); o != "" && (hosts == nil || !slices.Contains(hosts, strings.TrimPrefix(o, "http://"))) {
					http.Error(w, "cross-origin request", http.StatusForbidden)
					return
				}
			}
		}
	}
	switch {
	case strings.HasPrefix(r.URL.Path, "/rpc/"):
		s.rpc(w, r)
		return
	case r.URL.Path == "/events":
		s.events(w, r)
		return
	}
	for prefix, h := range s.opts.Routes {
		if strings.HasPrefix(r.URL.Path, prefix) {
			h.ServeHTTP(w, r)
			return
		}
	}
	if s.opts.Static != nil {
		s.opts.Static.ServeHTTP(w, r)
		return
	}
	http.NotFound(w, r)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

var errType = reflect.TypeFor[error]()
var ctxType = reflect.TypeFor[context.Context]()

func (s *Server) rpc(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "use POST"})
		return
	}
	// A cross-site form can post text/plain; it can't post JSON without
	// asking first (CORS), and we never say yes.
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "arguments must be sent as application/json"})
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/rpc/")
	m, ok := s.methods[name]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown method " + name})
		return
	}
	var raw []json.RawMessage
	if err := json.NewDecoder(io.LimitReader(r.Body, 256<<20)).Decode(&raw); err != nil && err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "arguments must be a JSON array"})
		return
	}
	mt := m.Type()
	first := 0
	if mt.NumIn() > 0 && mt.In(0) == ctxType {
		first = 1
	}
	if len(raw) != mt.NumIn()-first {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("%s takes %d arguments, got %d", name, mt.NumIn()-first, len(raw))})
		return
	}
	args := make([]reflect.Value, mt.NumIn())
	if first == 1 {
		args[0] = reflect.ValueOf(r.Context())
	}
	for i := first; i < len(args); i++ {
		v := reflect.New(mt.In(i))
		if err := json.Unmarshal(raw[i-first], v.Interface()); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("argument %d: %v", i-first, err)})
			return
		}
		args[i] = v.Elem()
	}
	out := m.Call(args)
	var result any
	for _, o := range out {
		if o.Type().Implements(errType) {
			if !o.IsNil() {
				writeJSON(w, http.StatusOK, map[string]string{"error": o.Interface().(error).Error()})
				return
			}
			continue
		}
		result = o.Interface()
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": result})
}

// Broadcast sends an event to every page.
func (s *Server) Broadcast(name string, data any) {
	msg, err := json.Marshal(map[string]any{"name": name, "data": data})
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for ch := range s.subs {
		select {
		case ch <- msg:
		default: // slow page: drop
		}
	}
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	ch := make(chan []byte, 256)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.subs, ch)
		s.mu.Unlock()
	}()
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.done:
			return
		case msg := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}
