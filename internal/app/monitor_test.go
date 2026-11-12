package app

import (
	"testing"
)

func TestDockTabsAndCollapse(t *testing.T) {
	a, _, tt := newRemoteDockApp(t)
	for _, want := range []string{"文件", "传输", "隧道", "命令", "文件名"} {
		if !tt.HasText(want) {
			t.Fatalf("dock missing %q; texts = %q", want, tt.Texts())
		}
	}

	a.toggleDock()
	tt.Frame()
	if a.bottomFiles {
		t.Fatal("dock should be collapsed")
	}
	if tt.HasText("文件名") {
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
