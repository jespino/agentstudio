package desktop

import (
	"testing"

	"github.com/jespino/agentstudio/studio"
)

// app is a studio's service, as the studios define it.
type app struct {
	*studio.Workspace
}

// Main and Run infer their type parameters from the service.
var _ = func() { Main(&app{}, Options{}) }

func TestBrowserOptions(t *testing.T) {
	open := func(string) error { return nil }
	b := Options{ID: "demo", Name: "Demo", Routes: []string{"/__demo/"}, Open: open, ServeAddr: "127.0.0.1:1"}.Browser()
	if b.ID != "demo" || b.Name != "Demo" || len(b.Routes) != 1 || b.Open == nil || b.Addr != "127.0.0.1:1" {
		t.Fatalf("%+v", b)
	}
}
