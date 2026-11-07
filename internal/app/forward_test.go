package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"xu/internal/sshx"
)

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
