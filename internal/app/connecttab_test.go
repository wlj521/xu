package app

import (
	"context"
	"testing"

	"github.com/egoist/mygo/ui"

	"xu/internal/sshx"
	"xu/internal/store"
)

// connectingTabTestApp opens a quick-connect page and starts a saved
// connection, so the test has the connecting tab the user would see.
func connectingTabTestApp(t *testing.T) (*App, *appTab) {
	t.Helper()
	a := New()
	a.stop() // never dial a real host
	p := quickTestProfile()
	a.profiles = []store.Profile{p}
	a.openQuickTab()
	a.connectSavedProfile(p)

	tab := a.activeTabRef()
	if tab == nil || tab.kind != tabConnecting || tab.conn == nil {
		t.Fatalf("clicking the card did not open a connecting tab: %+v", tab)
	}
	return a, tab
}

func TestConnectingTabShowsPhaseAndTarget(t *testing.T) {
	a, _ := connectingTabTestApp(t)
	a.connPhase = sshx.PhaseAuth

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	for _, want := range []string{
		"正在连接 alpha", "root@example.com:22", "正在验证账号与密码", "连接中",
		"连接成功后在这里显示 CPU、内存、磁盘与网络",
	} {
		if !tt.HasText(want) {
			t.Fatalf("connecting tab is missing %q; texts = %q", want, tt.Texts())
		}
	}
}

func TestConnectingTabFailureOffersRetry(t *testing.T) {
	a, tab := connectingTabTestApp(t)
	tab.conn.err = "密码错误"
	a.connecting = false
	a.connectingProfileID = 0

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	for _, want := range []string{"连接失败", "密码错误", "重试", "编辑连接"} {
		if !tt.HasText(want) {
			t.Fatalf("failure view is missing %q; texts = %q", want, tt.Texts())
		}
	}

	if err := tt.Click("重试"); err != nil {
		t.Fatalf("click 重试: %v", err)
	}
	if !a.connecting || a.connTabID != tab.id {
		t.Fatalf("retry did not start: connecting=%v tab=%d/%d", a.connecting, a.connTabID, tab.id)
	}
	if tab.conn == nil || tab.conn.err != "" {
		t.Fatalf("retry left the old failure behind: %+v", tab.conn)
	}
}

func TestConnectingTabFailureCanEditProfile(t *testing.T) {
	a, tab := connectingTabTestApp(t)
	tab.conn.err = "密码错误"
	a.connecting = false
	a.connectingProfileID = 0

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	if err := tt.Click("编辑连接"); err != nil {
		t.Fatalf("click 编辑连接: %v", err)
	}
	if !a.formOpen || a.profileID != 1 || a.host != "example.com" {
		t.Fatalf("edit did not load the profile: open=%v id=%d host=%q", a.formOpen, a.profileID, a.host)
	}
}

func TestClosingConnectingTabCancelsDial(t *testing.T) {
	a := New()
	a.stop()
	p := quickTestProfile()
	a.openQuickTab()
	tab := a.connectingTab(p)

	ctx, cancel := context.WithCancel(context.Background())
	tab.conn.cancel = cancel
	a.connecting = true
	a.connectingProfileID = p.ID
	a.connTabID = tab.id

	a.closeTabIndex(a.tabIndexByID(tab.id))

	if ctx.Err() == nil {
		t.Fatal("closing the connecting tab must cancel its dial")
	}
	if a.connecting || a.connTabID != 0 || a.connectingProfileID != 0 {
		t.Fatalf("connect state survived the closed tab: connecting=%v tab=%d profile=%d",
			a.connecting, a.connTabID, a.connectingProfileID)
	}
	if len(a.tabs) != 1 || a.tabs[0].kind != tabQuickConnect {
		t.Fatalf("tabs = %+v, want only the quick-connect page left", a.tabs)
	}
}

func TestCloseConnectOriginClosesOnlyItsQuickTab(t *testing.T) {
	a := New()
	a.stop()

	a.openQuickTab()
	origin := a.activeTabRef().id
	a.openQuickTab()
	other := a.activeTabRef().id
	tab := a.connectingTab(quickTestProfile())

	a.closeConnectOrigin(origin)

	if a.tabIndexByID(origin) >= 0 {
		t.Fatal("the quick-connect page the dial started from is still open")
	}
	if a.tabIndexByID(other) < 0 {
		t.Fatal("a quick-connect page opened later must stay open")
	}
	if a.tabIndexByID(tab.id) < 0 {
		t.Fatal("the connecting tab must stay open")
	}
	if a.activeTabRef() != tab {
		t.Fatalf("active tab changed to %+v, want the connecting tab", a.activeTabRef())
	}
}

func TestQuickConnectPageShowsRealPhase(t *testing.T) {
	a := qcTestApp()
	a.profiles = []store.Profile{{ID: 7, Name: "gamma", Host: "10.2.2.7", Port: 22, Username: "ops"}}
	a.connecting = true
	a.connectingProfileID = 7
	a.connPhase = sshx.PhaseKeyExchange

	tt := qcTestQuick(a, 1280, 800)
	tt.Frame()
	if !tt.HasText("正在协商加密算法") {
		t.Fatalf("quick connect page does not show the real phase; texts = %q", tt.Texts())
	}
}
