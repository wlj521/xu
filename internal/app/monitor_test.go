package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestDockTabsAndCollapse(t *testing.T) {
	a := New()
	a.formOpen = false
	a.terms = []*termSession{{ID: 1, Title: "PowerShell 7", local: true}}
	a.activeTerm = 0
	a.bottomFiles = true

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	for _, want := range []string{"文件", "传输", "隧道", "命令", "本地", "名称"} {
		if !tt.HasText(want) {
			t.Fatalf("dock missing %q; texts = %q", want, tt.Texts())
		}
	}

	a.toggleDock()
	tt.Frame()
	if a.bottomFiles {
		t.Fatal("dock should be collapsed")
	}
	if tt.HasText("名称") {
		t.Fatal("collapsed dock should hide its panel")
	}

	a.setDockTab(dockTransfers)
	tt.Frame()
	if !a.bottomFiles {
		t.Fatal("clicking a dock tab should expand it")
	}
	if !tt.HasText("暂无传输任务") {
		t.Fatalf("transfers panel missing; texts = %q", tt.Texts())
	}
}
