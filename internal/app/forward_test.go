package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"xu/internal/sshx"
)

func TestForwardListenDedupeAcrossSessions(t *testing.T) {
	a := New()
	if a.forwardListenInUse("127.0.0.1:8080") {
		t.Fatal("no forward is running yet")
	}
	a.forwards[1] = []*forwardEntry{{kind: 0, listen: "127.0.0.1:8080", target: "10.0.0.5:3306"}}
	if !a.forwardListenInUse("127.0.0.1:8080") {
		t.Fatal("an active forward must reserve its listen address")
	}
	if a.forwardListenInUse("127.0.0.1:8081") {
		t.Fatal("a different port must stay free")
	}
}

func TestTunnelPanelRendersInDock(t *testing.T) {
	a := New()
	a.formOpen = false
	a.terms = []*termSession{{
		ID:    1,
		Title: "test",
		profile: sshx.Profile{
			Host:     "test",
			Port:     22,
			Username: "root",
		},
	}}
	a.activeTerm = 0
	a.openForwards()

	tt := ui.NewTester(a.View, 1280, 800)
	tt.Frame()

	for _, want := range []string{"新建隧道", "本地", "远程", "SOCKS5", "启动隧道", "运行中的隧道"} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q; texts = %q", want, tt.Texts())
		}
	}
}
