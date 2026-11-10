package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestTabContextMenuItems(t *testing.T) {
	a := New()
	a.stop()
	a.terms = []*termSession{{ID: 1, Title: "alpha"}, {ID: 2, Title: "beta"}}
	a.syncTabs()

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	if err := tt.RightClick("标签页 1：alpha"); err != nil {
		t.Fatal(err)
	}
	menu := tt.Menu()
	for _, want := range []string{
		"复制标签", "连接", "连接全部", "断开", "关闭", "关闭其他", "关闭全部",
	} {
		found := false
		for _, item := range menu {
			if item == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("tab menu missing %q: %v", want, menu)
		}
	}
}

func TestTabContextMenuDuplicateQuickTab(t *testing.T) {
	a := New()
	a.stop()
	a.openQuickTab()

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	if err := tt.RightClick("标签页 1：新标签页"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("复制标签"); err != nil {
		t.Fatal(err)
	}
	if len(a.tabs) != 2 || a.tabs[1].kind != tabQuickConnect {
		t.Fatalf("duplicated quick tabs = %+v", a.tabs)
	}
}

func TestTabContextMenuDisconnectKeepsTab(t *testing.T) {
	a := New()
	a.stop()
	a.terms = []*termSession{{ID: 1, Title: "alpha"}}
	a.syncTabs()

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	if err := tt.RightClick("标签页 1：alpha"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("断开"); err != nil {
		t.Fatal(err)
	}
	if len(a.tabs) != 1 || len(a.terms) != 1 || !a.terms[0].exited {
		t.Fatalf("disconnect changed the tab list: tabs=%d terms=%d exited=%v",
			len(a.tabs), len(a.terms), a.terms[0].exited)
	}
}

func TestTabContextMenuCloseOthers(t *testing.T) {
	a := New()
	a.stop()
	a.terms = []*termSession{{ID: 1, Title: "alpha"}, {ID: 2, Title: "beta"}, {ID: 3, Title: "gamma"}}
	a.syncTabs()

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	if err := tt.RightClick("标签页 2：beta"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("关闭其他"); err != nil {
		t.Fatal(err)
	}
	if len(a.tabs) != 1 || a.tabTitle(a.activeTabRef()) != "beta" || len(a.terms) != 1 {
		t.Fatalf("close others: tabs=%d terms=%d active=%q",
			len(a.tabs), len(a.terms), a.tabTitle(a.activeTabRef()))
	}
}

func TestTabContextMenuCloseAll(t *testing.T) {
	a := New()
	a.stop()
	a.terms = []*termSession{{ID: 1, Title: "alpha"}, {ID: 2, Title: "beta"}}
	a.syncTabs()

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	if err := tt.RightClick("标签页 1：alpha"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("关闭全部"); err != nil {
		t.Fatal(err)
	}
	if len(a.tabs) != 0 || len(a.terms) != 0 {
		t.Fatalf("close all left tabs=%d terms=%d", len(a.tabs), len(a.terms))
	}
}
