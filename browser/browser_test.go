package browser

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jespino/agentstudio/rpc"
	"github.com/jespino/agentstudio/studio"
)

// app is a studio's service, as the studios define it.
type app struct {
	*studio.Workspace
}

func (a *app) ServeHTTP(w http.ResponseWriter, r *http.Request) { w.Write([]byte("own " + r.URL.Path)) }

func TestListen(t *testing.T) {
	a := &app{studio.NewWorkspace(studio.Config{ID: "demo", Ext: ".demo", RecentFile: filepath.Join(t.TempDir(), "r.json")}, studio.WorkspaceOptions{})}
	defer a.Shutdown()
	t.Setenv(rpc.TokenEnv, "dev-token")
	o := Options{ID: "demo", Name: "Demo", Assets: fstest.MapFS{"index.html": {Data: []byte("<h1>demo</h1>")}}, Routes: []string{"/__demo/"}}
	srv, start, err := Listen(a, o, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	if !strings.HasSuffix(start, "/?token=dev-token") {
		t.Fatalf("start URL %s", start)
	}
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	get := func(path string) string {
		t.Helper()
		r, err := c.Get(path)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		var b strings.Builder
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		b.Write(buf[:n])
		return b.String()
	}
	if page := get(start); !strings.Contains(page, "<h1>demo</h1>") {
		t.Fatalf("page %q", page)
	}
	u, _ := url.Parse(start)
	if got := get("http://" + u.Host + "/__demo/x"); got != "own /__demo/x" {
		t.Fatalf("route %q", got)
	}
	req, _ := http.NewRequest(http.MethodPost, "http://"+u.Host+"/rpc/Harnesses", strings.NewReader("[]"))
	req.Header.Set("Content-Type", "application/json")
	r, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	var out struct {
		Result []map[string]any `json:"result"`
	}
	if err := json.NewDecoder(r.Body).Decode(&out); err != nil || len(out.Result) == 0 || out.Result[0]["id"] != "claude" {
		t.Fatalf("Harnesses: %v %v", out, err)
	}
}
