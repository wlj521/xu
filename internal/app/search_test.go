package app

import (
	"testing"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

func TestSearchIndexConversion(t *testing.T) {
	a := New()
	a.formOpen = false
	a.terms = []*termSession{{ID: 7, Title: "test"}}
	a.activeTerm = 0
	a.finding = true
	a.searchInit = false

	a.onTermSearch(7, terminal.SearchStatus{Total: 3, Selected: 2})
	if a.searchTotal != 3 || a.searchIndex != 1 || a.lastIndex != 1 || !a.searchInit {
		t.Fatalf("search state = total %d index %d last %d init %v", a.searchTotal, a.searchIndex, a.lastIndex, a.searchInit)
	}
}

func TestReconnectBarRenders(t *testing.T) {
	a := New()
	a.formOpen = false
	a.terms = []*termSession{{ID: 1, Title: "test", exited: true}}
	a.activeTerm = 0

	tt := ui.NewTester(a.View, 1280, 800)
	tt.Frame()

	for _, want := range []string{"连接已断开", "重连"} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q; texts = %q", want, tt.Texts())
		}
	}
}
