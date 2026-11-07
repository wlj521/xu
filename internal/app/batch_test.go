package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"xu/internal/store"
)

func TestBatchPanelRendersInDock(t *testing.T) {
	a := New()
	a.formOpen = false
	a.profiles = []store.Profile{{ID: 1, Name: "test-server"}}
	a.terms = []*termSession{{ID: 1, Title: "本地终端", local: true}}
	a.activeTerm = 0
	a.bottomFiles = true
	a.openBatch()

	tt := ui.NewTester(a.View, 1280, 800)
	tt.Frame()

	for _, want := range []string{"批量命令", "目标 1 / 1", "执行", "执行结果会显示在这里"} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q; texts = %q", want, tt.Texts())
		}
	}
}
