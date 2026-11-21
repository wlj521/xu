package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"xu/internal/store"
)

func TestTerminalSplitMenuFocusAndClose(t *testing.T) {
	a := New()
	a.stop()
	a.terms = []*termSession{{ID: 1, Title: "alpha"}, {ID: 2, Title: "beta"}}
	a.syncTabs()

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	if _, ok := tt.Find("终端分屏"); !ok {
		t.Fatalf("terminal split action missing; texts = %q", tt.Texts())
	}
	if err := tt.Click("终端分屏"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"左右分屏", "上下分屏"} {
		if !containsString(tt.Menu(), want) {
			t.Fatalf("split menu missing %q: %v", want, tt.Menu())
		}
	}
	if err := tt.ChooseMenuItem("左右分屏", "2  beta"); err != nil {
		t.Fatal(err)
	}
	if a.terminalSplit == nil || a.terminalSplit.layout != splitLayoutColumns ||
		a.terminalSplit.firstID != a.tabs[0].id || a.terminalSplit.secondID != a.tabs[1].id {
		t.Fatalf("split state = %+v", a.terminalSplit)
	}

	tt.Frame()
	firstBox, ok := tt.Find("聚焦分屏面板 alpha")
	if !ok {
		t.Fatalf("first split pane missing; texts = %q", tt.Texts())
	}
	secondBox, ok := tt.Find("聚焦分屏面板 beta")
	if !ok {
		t.Fatalf("second split pane missing; texts = %q", tt.Texts())
	}
	if firstBox.X >= secondBox.X || firstBox.Y != secondBox.Y {
		t.Fatalf("left/right split laid out incorrectly: first=%+v second=%+v", firstBox, secondBox)
	}
	if err := tt.Click("聚焦分屏面板 beta"); err != nil {
		t.Fatal(err)
	}
	if a.activeTab != 1 || a.activeTerm != 1 || a.terminalSplit == nil {
		t.Fatalf("focusing the second pane changed the split: activeTab=%d activeTerm=%d split=%+v",
			a.activeTab, a.activeTerm, a.terminalSplit)
	}

	if err := tt.Click("终端分屏 · 左右分屏"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("切换为上下分屏"); err != nil {
		t.Fatal(err)
	}
	if a.terminalSplit == nil || a.terminalSplit.layout != splitLayoutRows {
		t.Fatalf("layout after toggle = %+v", a.terminalSplit)
	}
	tt.Frame()
	firstBox, ok = tt.Find("聚焦分屏面板 alpha")
	if !ok {
		t.Fatalf("first split pane missing after toggle; texts = %q", tt.Texts())
	}
	secondBox, ok = tt.Find("聚焦分屏面板 beta")
	if !ok {
		t.Fatalf("second split pane missing after toggle; texts = %q", tt.Texts())
	}
	if firstBox.Y >= secondBox.Y || firstBox.X != secondBox.X {
		t.Fatalf("top/bottom split laid out incorrectly: first=%+v second=%+v", firstBox, secondBox)
	}

	if err := tt.Click("终端分屏 · 上下分屏"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("关闭分屏"); err != nil {
		t.Fatal(err)
	}
	if a.terminalSplit != nil {
		t.Fatalf("split stayed after close: %+v", a.terminalSplit)
	}
}

func TestSwitchingToThirdTabLeavesTerminalSplit(t *testing.T) {
	a := New()
	a.stop()
	a.terms = []*termSession{
		{ID: 1, Title: "alpha"},
		{ID: 2, Title: "beta"},
		{ID: 3, Title: "gamma"},
	}
	a.syncTabs()
	a.startTerminalSplit(a.tabs[1].id, splitLayoutColumns)
	a.activateTabIndex(2)

	if a.terminalSplit != nil {
		t.Fatalf("split stayed after selecting a third tab: %+v", a.terminalSplit)
	}
	if a.activeTab != 2 || a.activeTerm != 2 {
		t.Fatalf("activeTab=%d activeTerm=%d, want gamma", a.activeTab, a.activeTerm)
	}
}

func TestClosingSplitTabClearsLayout(t *testing.T) {
	a := New()
	a.stop()
	a.terms = []*termSession{{ID: 1, Title: "alpha"}, {ID: 2, Title: "beta"}}
	a.syncTabs()
	a.startTerminalSplit(a.tabs[1].id, splitLayoutRows)
	a.closeTabIndex(1)

	if a.terminalSplit != nil {
		t.Fatalf("split stayed after closing a pane: %+v", a.terminalSplit)
	}
	if len(a.tabs) != 1 || a.tabTitle(a.activeTabRef()) != "alpha" {
		t.Fatalf("remaining tab = %+v", a.tabs)
	}
}

func TestTerminalSplitWorkspaceSnapshot(t *testing.T) {
	a := New()
	a.stop()
	a.profiles = []store.Profile{
		{ID: 11, Name: "alpha", Host: "alpha.example.com", Port: 22, Username: "root"},
		{ID: 22, Name: "beta", Host: "beta.example.com", Port: 22, Username: "root"},
	}
	a.terms = []*termSession{
		{ID: 1, Title: "alpha", profileID: 11},
		{ID: 2, Title: "beta", profileID: 22},
	}
	a.syncTabs()
	a.startTerminalSplit(a.tabs[1].id, splitLayoutRows)
	a.terminalSplit.ratio = 36

	state := a.workspaceSnapshot()
	if state.Split == nil {
		t.Fatal("split was not written to the workspace")
	}
	if state.Split.Layout != "rows" || state.Split.First != 0 ||
		state.Split.Second != 1 || state.Split.Ratio != 36 {
		t.Fatalf("workspace split = %+v", state.Split)
	}
}
