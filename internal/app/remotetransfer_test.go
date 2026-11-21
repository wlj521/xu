package app

import (
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"xu/internal/sshx"
	"xu/internal/store"
)

func TestRemoteTransferDialogQueuesTask(t *testing.T) {
	a := New()
	a.stop()
	a.profiles = []store.Profile{
		{ID: 11, Name: "alpha", Host: "alpha.example.com", Port: 22, Username: "root"},
		{ID: 22, Name: "beta", Host: "beta.example.com", Port: 22, Username: "root"},
	}
	a.terms = []*termSession{
		{
			ID: 1, Title: "alpha", profileID: 11,
			profile: sshx.Profile{Host: "alpha.example.com", Port: 22, Username: "root"},
		},
		{
			ID: 2, Title: "beta", profileID: 22,
			profile: sshx.Profile{Host: "beta.example.com", Port: 22, Username: "root"},
		},
	}
	a.syncTabs()
	b := &fileBrowser{
		termID:        1,
		remoteDir:     "/data",
		remoteEntries: []os.FileInfo{fakeInfo{name: "app.log", size: 10}},
		remoteSel:     0,
	}
	a.openRemoteTransferDialog(b)
	if !a.remoteTransferOpen {
		t.Fatal("remote transfer dialog did not open")
	}
	if a.remoteTransferTargetChoice == "" {
		t.Fatal("target server was not chosen")
	}
	a.remoteTransferTargetDir = "/srv"

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	for _, want := range []string{
		"发送到其他服务器", "alpha", "目标服务器", "目标目录", "同名冲突", "开始发送",
	} {
		if !tt.HasText(want) {
			t.Fatalf("remote transfer dialog missing %q; texts = %q", want, tt.Texts())
		}
	}
	if err := tt.Click("开始发送"); err != nil {
		t.Fatal(err)
	}
	if a.remoteTransferOpen {
		t.Fatal("dialog stayed open after queueing the task")
	}
	if len(a.transferTasks) != 1 {
		t.Fatalf("transfer tasks = %+v", a.transferTasks)
	}
	task := a.transferTasks[0]
	if task.kind != transferRemote || task.sessionID != 1 || task.targetSessionID != 2 {
		t.Fatalf("queued task = %+v", task)
	}
	if task.payload.Remote == nil ||
		task.payload.Remote.SourcePath != "/data/app.log" ||
		task.payload.Remote.TargetDir != "/srv" ||
		task.payload.Remote.TargetProfileID != 22 ||
		task.payload.Remote.Directory {
		t.Fatalf("remote payload = %+v", task.payload.Remote)
	}
	if !strings.Contains(task.target, "beta.example.com") || !strings.Contains(task.target, "/srv/app.log") {
		t.Fatalf("target label = %q", task.target)
	}
	if task.status != transferFailed {
		t.Fatalf("unconnected test session did not fail fast: %+v", task)
	}
}

func TestRemoteTransferNeedsAnotherConnectedTab(t *testing.T) {
	a := New()
	a.stop()
	a.terms = []*termSession{{
		ID: 1, Title: "alpha",
		profile: sshx.Profile{Host: "alpha.example.com", Port: 22, Username: "root"},
	}}
	a.syncTabs()
	b := &fileBrowser{
		termID:        1,
		remoteDir:     "/",
		remoteEntries: []os.FileInfo{fakeInfo{name: "app.log", size: 10}},
		remoteSel:     0,
	}
	a.openRemoteTransferDialog(b)

	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	if !tt.HasText("没有其他已连接的 SSH 标签") {
		t.Fatalf("dialog did not explain the missing target; texts = %q", tt.Texts())
	}
	_ = tt.Click("开始发送")
	if !a.remoteTransferOpen || len(a.transferTasks) != 0 {
		t.Fatalf("disabled send queued a task: open=%v tasks=%+v", a.remoteTransferOpen, a.transferTasks)
	}
}
