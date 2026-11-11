package app

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"xu/internal/store"
)

// qcTestApp is an App whose context is already cancelled, so the connect
// paths these tests trigger never dial a real host.
func qcTestApp() *App {
	a := New()
	a.stop()
	return a
}

func qcTestQuick(a *App, width, height int) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		theme := a.applyTheme(c)
		a.quickConnectView(c, theme)
	}, width, height)
}

func qcTestManager(a *App, width, height int) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		theme := a.applyTheme(c)
		a.connectionManagerModal(c, theme)
	}, width, height)
}

func TestQuickConnectPageShowsSavedConnections(t *testing.T) {
	a := qcTestApp()
	a.groups = []store.Group{{ID: 2, Name: "生产环境"}}
	a.profiles = []store.Profile{
		{ID: 1, Name: "alpha", Host: "10.0.0.1", Port: 22, Username: "root", AuthType: "password", GroupID: 2},
		{ID: 2, Name: "beta", Host: "10.0.0.2", Port: 2222, Username: "deploy", AuthType: "key"},
	}

	tt := qcTestQuick(a, 1280, 800)
	tt.Frame()

	for _, want := range []string{
		"快速连接", "生产环境", "alpha", "root@10.0.0.1:22",
		"beta", "deploy@10.0.0.2:2222", "未分组", "连接管理",
	} {
		if !tt.HasText(want) {
			t.Fatalf("quick connect page is missing %q; texts = %q", want, tt.Texts())
		}
	}

	if err := tt.Click("alpha"); err != nil {
		t.Fatalf("click the alpha card: %v", err)
	}
	if !a.connecting {
		t.Fatal("clicking a saved connection must start a connection")
	}
	if a.connectingProfileID != 1 {
		t.Fatalf("connecting profile = %d, want 1", a.connectingProfileID)
	}
	if a.formOpen {
		t.Fatal("the quick connect page must dial directly instead of opening the form")
	}
}

func TestQuickConnectPageEmptyState(t *testing.T) {
	a := qcTestApp()

	tt := qcTestQuick(a, 1280, 800)
	tt.Frame()

	if !tt.HasText("还没有保存的连接") {
		t.Fatalf("empty state missing; texts = %q", tt.Texts())
	}
	if !tt.HasText("本机终端") {
		t.Fatalf("empty state must offer the local terminal; texts = %q", tt.Texts())
	}

	if err := tt.Click("新建连接"); err != nil {
		t.Fatalf("click 新建连接: %v", err)
	}
	if !a.formOpen {
		t.Fatal("the empty state must open the connection form")
	}
}

func TestQuickConnectPageSearchFiltersCards(t *testing.T) {
	a := qcTestApp()
	a.profiles = []store.Profile{
		{ID: 1, Name: "alpha", Host: "10.0.0.1", Port: 22, Username: "root"},
		{ID: 2, Name: "beta", Host: "10.0.0.2", Port: 22, Username: "deploy"},
	}
	a.quickFilter = "beta"

	tt := qcTestQuick(a, 1280, 800)
	tt.Frame()
	if !tt.HasText("deploy@10.0.0.2:22") {
		t.Fatalf("matching connection is missing; texts = %q", tt.Texts())
	}
	if tt.HasText("root@10.0.0.1:22") {
		t.Fatal("the search box did not hide the other connections")
	}

	a.quickFilter = "no-such-server"
	tt = qcTestQuick(a, 1280, 800)
	tt.Frame()
	if !tt.HasText("没有匹配") {
		t.Fatalf("missing the no-match state; texts = %q", tt.Texts())
	}
	if err := tt.Click("清除筛选"); err != nil {
		t.Fatalf("click 清除筛选: %v", err)
	}
	tt.Frame()
	if a.quickFilter != "" {
		t.Fatalf("quick filter = %q, want it cleared", a.quickFilter)
	}
}

func TestQuickConnectPageShowsProgressAndFailure(t *testing.T) {
	a := qcTestApp()
	a.profiles = []store.Profile{{ID: 7, Name: "gamma", Host: "10.2.2.7", Port: 22, Username: "ops"}}
	a.connecting = true
	a.connectingProfileID = 7

	tt := qcTestQuick(a, 1280, 800)
	tt.Frame()
	if !tt.HasText("正在连接 gamma") {
		t.Fatalf("connecting state is not visible; texts = %q", tt.Texts())
	}

	a.connecting = false
	a.connectErr = "密钥认证失败"
	tt = qcTestQuick(a, 1280, 800)
	tt.Frame()
	if !tt.HasText("连接失败") || !tt.HasText("密钥认证失败") {
		t.Fatalf("failure state is not visible; texts = %q", tt.Texts())
	}
	if err := tt.Click("关闭提示"); err != nil {
		t.Fatalf("click 关闭提示: %v", err)
	}
	if a.connectErr != "" {
		t.Fatalf("connect error = %q, want it dismissed", a.connectErr)
	}
}

func TestConnectionManagerModalListsAndEdits(t *testing.T) {
	a := qcTestApp()
	a.groups = []store.Group{{ID: 3, Name: "生产环境"}, {ID: 4, Name: "空分组"}}
	a.profiles = []store.Profile{
		{ID: 9, Name: "gamma", Host: "10.1.1.9", Port: 2200, Username: "ops", AuthType: "password", GroupID: 3},
	}
	a.managerOpen = true
	a.managerSel = 9

	tt := qcTestManager(a, 1280, 800)
	tt.Frame()

	for _, want := range []string{
		"连接管理器", "生产环境", "空分组", "gamma", "ops@10.1.1.9:2200",
		"新建连接", "新建分组", "关闭",
	} {
		if !tt.HasText(want) {
			t.Fatalf("connection manager is missing %q; texts = %q", want, tt.Texts())
		}
	}

	if err := tt.Click("编辑连接 gamma"); err != nil {
		t.Fatalf("click the inline edit action: %v", err)
	}
	if a.managerOpen {
		t.Fatal("the manager must close before the connection form opens")
	}
	if !a.formOpen || a.profileID != 9 || a.host != "10.1.1.9" || a.port != "2200" {
		t.Fatalf("edit context = open:%v id:%d host:%q port:%q",
			a.formOpen, a.profileID, a.host, a.port)
	}
}

func TestConnectionManagerConnectsARow(t *testing.T) {
	a := qcTestApp()
	a.profiles = []store.Profile{
		{ID: 11, Name: "delta", Host: "10.3.3.11", Port: 2222, Username: "web", AuthType: "key"},
	}
	a.managerOpen = true

	tt := qcTestManager(a, 1280, 800)
	tt.Frame()

	if err := tt.Click("连接 delta"); err != nil {
		t.Fatalf("click the inline connect action: %v", err)
	}
	if !a.connecting || a.connectingProfileID != 11 {
		t.Fatalf("connecting=%v profile=%d, want the delta connection", a.connecting, a.connectingProfileID)
	}
	if a.managerOpen {
		t.Fatal("the manager must close once a connection starts")
	}
}

func TestConnectionManagerSearchFiltersRows(t *testing.T) {
	a := qcTestApp()
	a.profiles = []store.Profile{
		{ID: 1, Name: "alpha", Host: "10.0.0.1", Port: 22, Username: "root"},
		{ID: 2, Name: "beta", Host: "10.0.0.2", Port: 22, Username: "deploy"},
	}
	a.managerOpen = true
	a.managerFilter = "beta"

	tt := qcTestManager(a, 1280, 800)
	tt.Frame()
	if !tt.HasText("deploy@10.0.0.2:22") {
		t.Fatalf("matching row is missing; texts = %q", tt.Texts())
	}
	if tt.HasText("root@10.0.0.1:22") {
		t.Fatal("the manager search box did not hide the other rows")
	}

	a.managerFilter = "no-such-server"
	tt = qcTestManager(a, 1280, 800)
	tt.Frame()
	if !tt.HasText("没有匹配") {
		t.Fatalf("missing the manager no-match state; texts = %q", tt.Texts())
	}
}

func TestConnectionManagerGroupForm(t *testing.T) {
	a := qcTestApp()
	a.profiles = []store.Profile{
		{ID: 1, Name: "alpha", Host: "10.0.0.1", Port: 22, Username: "root"},
	}
	a.managerOpen = true

	tt := qcTestManager(a, 1280, 800)
	tt.Frame()
	if err := tt.Click("新建分组"); err != nil {
		t.Fatalf("click 新建分组: %v", err)
	}
	if !a.groupFormOpen {
		t.Fatal("新建分组 must open the inline group form")
	}
	tt.Frame()
	if !tt.HasText("保存") || !tt.HasText("取消") {
		t.Fatalf("group form is missing its actions; texts = %q", tt.Texts())
	}

	if err := tt.Click("保存"); err != nil {
		t.Fatalf("click 保存: %v", err)
	}
	if !strings.Contains(a.status, "分组名称不能为空") {
		t.Fatalf("status = %q, want the empty-name warning", a.status)
	}

	if err := tt.Click("取消"); err != nil {
		t.Fatalf("click 取消: %v", err)
	}
	tt.Frame()
	if a.groupFormOpen {
		t.Fatal("取消 must close the group form")
	}
}

func TestQuickConnectAndManagerPaintInBothThemes(t *testing.T) {
	for _, mode := range []int{1, 2} {
		mode := mode
		a := qcTestApp()
		a.settings.Theme = mode
		a.profiles = []store.Profile{
			{ID: 1, Name: "alpha", Host: "10.0.0.1", Port: 22, Username: "root", AuthType: "key"},
		}

		var dark bool
		tt := ui.NewTester(func(c *ui.Context) {
			theme := a.applyTheme(c)
			dark = theme.Dark
			a.quickConnectView(c, theme)
		}, 1280, 800)
		tt.Frame()
		if dark != (mode == 1) {
			t.Fatalf("theme mode %d rendered dark=%v", mode, dark)
		}
		if !tt.HasText("alpha") {
			t.Fatalf("theme mode %d lost the connection cards; texts = %q", mode, tt.Texts())
		}

		var managerDark bool
		a.managerOpen = true
		mt := ui.NewTester(func(c *ui.Context) {
			theme := a.applyTheme(c)
			managerDark = theme.Dark
			a.connectionManagerModal(c, theme)
		}, 1280, 800)
		mt.Frame()
		if managerDark != (mode == 1) {
			t.Fatalf("manager in theme mode %d rendered dark=%v", mode, managerDark)
		}
		if !mt.HasText("连接管理器") || !mt.HasText("alpha") {
			t.Fatalf("manager in theme mode %d is empty; texts = %q", mode, mt.Texts())
		}
	}
}
