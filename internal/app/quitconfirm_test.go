package app

import (
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestQuitConfirmationOnlyWithOpenTabs(t *testing.T) {
	a := New()
	a.stop()

	empty := &mygo.QuitEvent{}
	a.handleBeforeQuit(empty)
	if empty.DefaultPrevented() {
		t.Fatal("an empty workspace asked for confirmation")
	}

	a.tabs = []*appTab{{id: 1, kind: tabQuickConnect}}
	quit := &mygo.QuitEvent{}
	a.handleBeforeQuit(quit)
	if !quit.DefaultPrevented() || !a.quitConfirmOpen {
		t.Fatalf("quit with tabs: prevented=%v modal=%v", quit.DefaultPrevented(), a.quitConfirmOpen)
	}
}

func TestWindowCloseStopsForQuitConfirmation(t *testing.T) {
	a := New()
	a.stop()
	a.tabs = []*appTab{{id: 1, kind: tabTerminal, term: &termSession{ID: 1, Title: "alpha"}}}

	e := &mygo.CloseEvent{}
	a.handleWindowClose(e)
	if !e.DefaultPrevented() || !a.quitConfirmOpen {
		t.Fatalf("window close: prevented=%v modal=%v", e.DefaultPrevented(), a.quitConfirmOpen)
	}
}

func TestQuitConfirmationRendersAndCancels(t *testing.T) {
	a := New()
	a.stop()
	a.terms = []*termSession{{ID: 1, Title: "alpha"}, {ID: 2, Title: "beta"}}
	a.syncTabs()
	a.quitConfirmOpen = true

	tt := ui.NewTester(a.View, 1280, 800)
	tt.Frame()
	for _, want := range []string{
		"退出 Xu？", "还有 2 个未关闭的标签页", "退出会断开全部 SSH 连接",
		"下次启动会恢复这些标签，但不会自动连接", "取消", "退出并断开",
	} {
		if !tt.HasText(want) {
			t.Fatalf("quit dialog missing %q; texts = %q", want, tt.Texts())
		}
	}
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	if a.quitConfirmOpen || a.quitAllowed {
		t.Fatalf("cancel left quit state open=%v allowed=%v", a.quitConfirmOpen, a.quitAllowed)
	}
}

func TestQuitConfirmationProceedsAfterConfirm(t *testing.T) {
	a := New()
	a.stop()
	a.tabs = []*appTab{{id: 1, kind: tabTerminal, term: &termSession{ID: 1, Title: "alpha"}}}
	a.quitConfirmOpen = true

	tt := ui.NewTester(a.View, 1280, 800)
	tt.Frame()
	if err := tt.Click("退出并断开"); err != nil {
		t.Fatal(err)
	}
	if a.quitConfirmOpen || !a.quitAllowed || !a.quitRequested {
		t.Fatalf("confirm state: open=%v allowed=%v requested=%v",
			a.quitConfirmOpen, a.quitAllowed, a.quitRequested)
	}
}

func TestSessionEndingSkipsQuitConfirmation(t *testing.T) {
	a := New()
	a.stop()
	a.tabs = []*appTab{{id: 1, kind: tabQuickConnect}}

	e := &mygo.QuitEvent{SessionEnding: true}
	a.handleBeforeQuit(e)
	if e.DefaultPrevented() || a.quitConfirmOpen || !a.quitInProgress {
		t.Fatalf("session ending: prevented=%v modal=%v progress=%v",
			e.DefaultPrevented(), a.quitConfirmOpen, a.quitInProgress)
	}
}
