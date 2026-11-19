package app

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"xu/internal/store"
)

func newQuickCommandDockApp(t *testing.T) (*App, *ui.Tester) {
	t.Helper()
	a, _, tt := newRemoteDockApp(t)
	a.terms[0].profileID = 7
	a.profiles = []store.Profile{{ID: 7, Name: "jt808"}}
	a.quickCommands = []store.QuickCommand{
		{ID: 1, ProfileID: 7, Name: "进入项目", Command: "cd /projects/jt808"},
		{ID: 2, ProfileID: 7, Name: "查看日志", Command: "cd /projects/jt808 && tail -f logs/app.log"},
		{ID: 3, Name: "uptime", Command: "uptime"},
	}
	a.quickCmdOpen = true
	tt.Frame()
	return a, tt
}

func TestQuickCommandButtonPrecedesFind(t *testing.T) {
	_, tt := newQuickCommandDockApp(t)

	button, ok := tt.Find("快捷命令")
	if !ok {
		t.Fatalf("quick command button missing; texts = %q", tt.Texts())
	}
	find, ok := tt.Find("查找（Ctrl Shift F）")
	if !ok {
		t.Fatalf("find button missing; texts = %q", tt.Texts())
	}
	if button.X > find.X {
		t.Fatalf("quick command button at %.1f is not before find at %.1f", button.X, find.X)
	}
	for _, want := range []string{
		"当前服务器 · jt808",
		"全局 · 所有服务器",
		"快捷命令 进入项目",
		"快捷命令 查看日志",
		"快捷命令 uptime",
		"点击只填入终端，按 Enter 执行",
	} {
		if !tt.HasText(want) {
			t.Fatalf("quick command popover missing %q; texts = %q", want, tt.Texts())
		}
	}
}

func TestQuickCommandEditOpensEditor(t *testing.T) {
	a, tt := newQuickCommandDockApp(t)

	if err := tt.Click("编辑快捷命令 进入项目"); err != nil {
		t.Fatalf("edit quick command: %v", err)
	}
	tt.Frame()
	if !a.quickCmdEditOpen || a.quickCmdID != 1 {
		t.Fatalf("editor state = open:%v id:%d", a.quickCmdEditOpen, a.quickCmdID)
	}
	for _, want := range []string{"编辑快捷命令", "名称", "命令", "当前服务器", "全局", "保存"} {
		if !tt.HasText(want) {
			t.Fatalf("quick command editor missing %q; texts = %q", want, tt.Texts())
		}
	}
}

func TestQuickCommandEditorValidatesScopeAndSingleLine(t *testing.T) {
	a := New()
	a.stop()
	a.quickCommands = []store.QuickCommand{{ID: 1, ProfileID: 7, Name: "uptime", Command: "uptime"}}
	a.quickCmdName = "uptime"
	a.quickCmdCommand = "uptime -p"
	a.quickCmdScope = quickCmdScopeServer
	a.quickCmdProfileID = 7
	a.saveQuickCommandEditor()
	if !strings.Contains(a.quickCmdErr, "同名") {
		t.Fatalf("duplicate name error = %q", a.quickCmdErr)
	}

	a.quickCmdName = "查看日志"
	a.quickCmdCommand = "cd /projects/jt808\ntail -f logs/app.log"
	a.quickCmdErr = ""
	a.saveQuickCommandEditor()
	if !strings.Contains(a.quickCmdErr, "多行") {
		t.Fatalf("multiline error = %q", a.quickCmdErr)
	}
}
