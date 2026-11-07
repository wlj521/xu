package app

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"xu/internal/store"
)

func TestSidebarProfileConnectsDirectly(t *testing.T) {
	a := New()
	a.stop() // keep the test from dialing a real host
	a.profiles = []store.Profile{{
		ID:       1,
		Name:     "alpha",
		Host:     "example.com",
		Port:     22,
		Username: "root",
	}}

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()

	if err := tt.Click("alpha"); err != nil {
		t.Fatal(err)
	}
	if a.formOpen {
		t.Fatal("clicking a saved profile must connect directly, not open the form")
	}
	if !a.connecting || a.profileID != 1 || a.host != "example.com" || a.username != "root" {
		t.Fatalf("connecting=%v profileID=%d host=%q user=%q", a.connecting, a.profileID, a.host, a.username)
	}
	if !strings.Contains(a.status, "正在连接") {
		t.Fatalf("status = %q", a.status)
	}
}

func TestConnectionManagerMergedIntoSidebar(t *testing.T) {
	a := New()
	a.stop()
	a.profiles = []store.Profile{{
		ID:       1,
		Name:     "alpha",
		Host:     "example.com",
		Port:     22,
		Username: "root",
		AuthType: "password",
	}}
	a.openConnectionManager()

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	if !tt.HasText("alpha") {
		t.Fatalf("sidebar missing connection; texts = %q", tt.Texts())
	}
	if !strings.Contains(a.status, "左侧连接树") {
		t.Fatalf("status = %q", a.status)
	}
	if err := tt.Click("alpha"); err != nil {
		t.Fatal(err)
	}
	if !a.connecting || a.profileID != 1 {
		t.Fatalf("connecting=%v profileID=%d", a.connecting, a.profileID)
	}
}
