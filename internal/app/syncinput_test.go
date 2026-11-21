package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSyncInputBytes(t *testing.T) {
	cases := []struct {
		name string
		ev   ui.InputEvent
		want []byte
	}{
		{"text", ui.InputEvent{Kind: ui.InputText, Text: "echo hi"}, []byte("echo hi")},
		{"enter", ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyEnter}, []byte("\r")},
		{"ctrl-c", ui.InputEvent{Kind: ui.InputKeyDown, Mods: ui.Ctrl, Key: ui.KeyC}, []byte{3}},
		{"shift-tab", ui.InputEvent{Kind: ui.InputKeyDown, Mods: ui.Shift, Key: ui.KeyTab}, []byte("\x1b[Z")},
		{"up", ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyUp}, []byte("\x1b[A")},
		{"f5", ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyF5}, []byte("\x1b[15~")},
		{"printable-keydown", ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyA}, nil},
		{"space-keydown", ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeySpace}, nil},
		{"command", ui.InputEvent{Kind: ui.InputKeyDown, Mods: ui.Ctrl, Key: ui.KeyB}, []byte{2}},
		{"copy-shortcut", ui.InputEvent{Kind: ui.InputKeyDown, Mods: ui.Ctrl | ui.Shift, Key: ui.KeyC}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := syncInputBytes(tc.ev); !bytes.Equal(got, tc.want) {
				t.Fatalf("bytes = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSyncInputBroadcastsToSelectedSSHOnly(t *testing.T) {
	a := New()
	a.stop()
	var sourceGot, targetGot, localGot []byte
	source := &termSession{
		ID: 1, Title: "alpha",
		writeInput: func(p []byte) { sourceGot = append(sourceGot, p...) },
	}
	target := &termSession{
		ID: 2, Title: "beta",
		writeInput: func(p []byte) { targetGot = append(targetGot, p...) },
	}
	local := &termSession{
		ID: 3, Title: "local", local: true,
		writeInput: func(p []byte) { localGot = append(localGot, p...) },
	}
	a.terms = []*termSession{source, target, local}
	a.syncTabs()
	a.activeTerm = 0
	a.syncInputEnabled = true
	a.syncInputIDs = map[int64]bool{1: true, 2: true, 3: true}

	a.onTerminalInput(source, ui.InputEvent{Kind: ui.InputText, Text: "echo hi"})
	a.onTerminalInput(source, ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyEnter})
	if string(sourceGot) != "" {
		t.Fatalf("source received mirrored input: %q", sourceGot)
	}
	if string(targetGot) != "echo hi\r" {
		t.Fatalf("target received %q", targetGot)
	}
	if string(localGot) != "" {
		t.Fatalf("local terminal received mirrored input: %q", localGot)
	}

	a.activeTerm = 1
	a.onTerminalInput(source, ui.InputEvent{Kind: ui.InputText, Text: "ignored"})
	if string(targetGot) != "echo hi\r" {
		t.Fatalf("background source broadcast input: %q", targetGot)
	}
}

func TestSyncInputDisablesWhenGroupBecomesTooSmall(t *testing.T) {
	a := New()
	a.stop()
	one := &termSession{ID: 1, Title: "alpha"}
	two := &termSession{ID: 2, Title: "beta"}
	a.terms = []*termSession{one, two}
	a.syncTabs()
	a.activeTerm = 0
	a.syncInputEnabled = true
	a.syncInputIDs = map[int64]bool{1: true, 2: true}

	a.closeTabIndex(1)
	if a.syncInputEnabled {
		t.Fatal("sync input stayed enabled after one of two tabs closed")
	}
	if a.syncInputSelected(2) {
		t.Fatal("closed session stayed selected")
	}
	if !strings.Contains(a.status, "自动关闭") {
		t.Fatalf("status = %q", a.status)
	}
}

func TestSyncInputMenuAndBanner(t *testing.T) {
	a := New()
	a.stop()
	a.terms = []*termSession{{ID: 1, Title: "alpha"}, {ID: 2, Title: "beta"}}
	a.syncTabs()
	a.activeTerm = 0

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	if _, ok := tt.Find("同步输入"); !ok {
		t.Fatalf("sync input action missing; texts = %q", tt.Texts())
	}
	if err := tt.Click("同步输入"); err != nil {
		t.Fatal(err)
	}
	menu := tt.Menu()
	for _, want := range []string{"高风险：相同的键盘输入会发送到多个服务器", "启用同步输入（已选 0 个）", "1  alpha", "2  beta"} {
		if !containsString(menu, want) {
			t.Fatalf("sync menu missing %q: %v", want, menu)
		}
	}
	if err := tt.ChooseMenuItem("全选 SSH 标签"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := a.syncInputCount(); got != 2 {
		t.Fatalf("selected sessions = %d, want 2", got)
	}

	if err := tt.Click("同步输入"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("启用同步输入（已选 2 个）"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.syncInputEnabled {
		t.Fatal("sync input did not enable")
	}
	for _, want := range []string{"同步输入已启用", "请确保所有标签处于相同交互状态", "关闭同步"} {
		if !tt.HasText(want) {
			t.Fatalf("sync banner missing %q; texts = %q", want, tt.Texts())
		}
	}
	if err := tt.Click("关闭同步"); err != nil {
		t.Fatal(err)
	}
	if a.syncInputEnabled {
		t.Fatal("closing the banner left sync input enabled")
	}
}

func containsString(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}
