package rpc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
)

type svc struct{ shut bool }

func (s *svc) Add(a, b int) int                         { return a + b }
func (s *svc) Fail() error                              { return errors.New("nope") }
func (s *svc) Ctx(ctx context.Context, x string) string { return x + "!" }
func (s *svc) SetEmitter(fn func(string, any))          {}
func (s *svc) SetThing(x any)                           {}
func (s *svc) Shutdown()                                { s.shut = true }
func (s *svc) Internal(x string)                        {}
func (s *svc) HiddenMethods() []string                  { return []string{"Internal"} }
func (s *svc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("route " + r.URL.Path))
}

func TestMethods(t *testing.T) {
	var names []string
	for n := range Methods(&svc{}) {
		names = append(names, n)
	}
	for _, want := range []string{"Add", "Fail", "Ctx"} {
		if _, ok := Methods(&svc{})[want]; !ok {
			t.Errorf("%s should be an RPC", want)
		}
	}
	for _, hidden := range []string{"SetEmitter", "SetThing", "Shutdown", "Internal", "HiddenMethods", "ServeHTTP"} {
		if _, ok := Methods(&svc{})[hidden]; ok {
			t.Errorf("%s should not be an RPC (got %v)", hidden, names)
		}
	}
}

func TestServer(t *testing.T) {
	s := &svc{}
	srv := New(s, Options{Name: "demo", Token: "secret", Routes: map[string]http.Handler{"/__demo/": s}})
	defer srv.Close()
	start, err := srv.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(start)
	base := "http://" + u.Host
	call := func(c *http.Client, method, body string, header map[string]string) (int, map[string]any) {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPost, base+"/rpc/"+method, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		for k, v := range header {
			req.Header.Set(k, v)
		}
		r, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		var out map[string]any
		json.NewDecoder(r.Body).Decode(&out)
		return r.StatusCode, out
	}

	// No token: nothing.
	if code, _ := call(http.DefaultClient, "Add", "[1,2]", nil); code != http.StatusForbidden {
		t.Fatalf("no token: %d", code)
	}
	if r, _ := http.Get(base + "/?token=wrong"); r.StatusCode != http.StatusForbidden {
		t.Fatalf("bad token: %d", r.StatusCode)
	}
	// Another name for our address (DNS rebinding).
	req, _ := http.NewRequest(http.MethodGet, start, nil)
	req.Host = "evil.example:" + u.Port()
	if r, _ := http.DefaultClient.Do(req); r.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign host: %d", r.StatusCode)
	}

	// The start URL sets the cookie.
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	if r, err := c.Get(start); err != nil || r.StatusCode != http.StatusNotFound { // no Static: 404 after the redirect
		t.Fatalf("start URL: %v %v", r.StatusCode, err)
	}
	if code, out := call(c, "Add", "[1,2]", nil); code != 200 || out["result"] != 3.0 {
		t.Fatalf("Add: %d %v", code, out)
	}
	if _, out := call(c, "Fail", "[]", nil); out["error"] != "nope" {
		t.Fatalf("Fail: %v", out)
	}
	if _, out := call(c, "Ctx", `["hi"]`, nil); out["result"] != "hi!" {
		t.Fatalf("Ctx: %v", out)
	}
	if code, _ := call(c, "Add", "[1]", nil); code != http.StatusBadRequest {
		t.Fatalf("wrong arity: %d", code)
	}
	if code, _ := call(c, "Shutdown", "[]", nil); code != http.StatusNotFound || s.shut {
		t.Fatalf("Shutdown is reachable: %d", code)
	}
	// With the cookie, other origins and non-JSON bodies are refused.
	if code, _ := call(c, "Add", "[1,2]", map[string]string{"Origin": "http://evil.example"}); code != http.StatusForbidden {
		t.Fatalf("cross-origin: %d", code)
	}
	if code, _ := call(c, "Add", "[1,2]", map[string]string{"Content-Type": "text/plain"}); code != http.StatusUnsupportedMediaType {
		t.Fatalf("text/plain: %d", code)
	}
	// The header token is for a development proxy: any origin.
	if code, out := call(http.DefaultClient, "Add", "[2,2]", map[string]string{TokenHeader: "secret", "Origin": "http://127.0.0.1:5173"}); code != 200 || out["result"] != 4.0 {
		t.Fatalf("header token: %d %v", code, out)
	}

	// Routes and events.
	r, err := c.Get(base + "/__demo/x")
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	bufio.NewReader(r.Body).WriteTo(&b)
	r.Body.Close()
	if b.String() != "route /__demo/x" {
		t.Fatalf("route: %q", b.String())
	}
	ev, err := c.Get(base + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer ev.Body.Close()
	lines := bufio.NewReader(ev.Body)
	lines.ReadString('\n') // ": connected"
	lines.ReadString('\n')
	srv.Broadcast("hello", map[string]int{"n": 1})
	line, _ := lines.ReadString('\n')
	if line != `data: {"data":{"n":1},"name":"hello"}`+"\n" {
		t.Fatalf("event: %q", line)
	}
}
