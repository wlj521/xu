package app

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"xu/internal/sshx"
	"xu/internal/store"
)

func quickTestProfile() store.Profile {
	return store.Profile{
		ID:       1,
		Name:     "alpha",
		Host:     "example.com",
		Port:     22,
		Username: "root",
		AuthType: "password",
	}
}

func TestQuickConnectCardConnectsInCurrentTab(t *testing.T) {
	a := New()
	a.stop() // keep the test from dialing a real host
	a.profiles = []store.Profile{quickTestProfile()}
	a.openQuickTab()

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	if !tt.HasText("快速连接") {
		t.Fatalf("quick connect page missing; texts = %q", tt.Texts())
	}
	if !tt.HasText("alpha") {
		t.Fatalf("saved connection missing; texts = %q", tt.Texts())
	}

	if err := tt.Click("alpha"); err != nil {
		t.Fatal(err)
	}
	if !a.connecting || a.profileID != 1 || a.host != "example.com" || a.username != "root" {
		t.Fatalf("connecting=%v profileID=%d host=%q user=%q", a.connecting, a.profileID, a.host, a.username)
	}
	if !strings.Contains(a.status, "正在连接") {
		t.Fatalf("status = %q", a.status)
	}
	tab := a.activeTabRef()
	if a.pendingQuickTab == 0 || tab == nil || tab.kind != tabQuickConnect || tab.id != a.pendingQuickTab {
		t.Fatalf("quick tab should host the session: pending=%d tab=%+v", a.pendingQuickTab, tab)
	}
}

func TestConnectionManagerListsProfiles(t *testing.T) {
	a := New()
	a.stop()
	a.profiles = []store.Profile{quickTestProfile()}
	a.openConnectionManager()

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	for _, want := range []string{"连接管理器", "alpha", "新建连接", "新建分组", "关闭"} {
		if !tt.HasText(want) {
			t.Fatalf("connection manager missing %q; texts = %q", want, tt.Texts())
		}
	}

	if err := tt.Click("新建分组"); err != nil {
		t.Fatal(err)
	}
	if !a.groupFormOpen {
		t.Fatal("new group form did not open from the manager")
	}
}

func TestTopTabsSwitchAndCloseSessions(t *testing.T) {
	a := New()
	a.stop()
	a.terms = []*termSession{{ID: 1, Title: "alpha"}, {ID: 2, Title: "beta"}}
	a.syncTabs()
	if len(a.tabs) != 2 || a.activeTab != 0 {
		t.Fatalf("tabs=%d active=%d", len(a.tabs), a.activeTab)
	}

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	if err := tt.Click("beta"); err != nil {
		t.Fatal(err)
	}
	if a.activeTab != 1 || a.activeTerm != 1 {
		t.Fatalf("tab click did not focus beta: activeTab=%d activeTerm=%d", a.activeTab, a.activeTerm)
	}

	if err := tt.Click("关闭 beta"); err != nil {
		t.Fatal(err)
	}
	if len(a.tabs) != 1 || len(a.terms) != 1 {
		t.Fatalf("tabs=%d terms=%d", len(a.tabs), len(a.terms))
	}
	if a.activeTabRef() == nil || a.tabTitle(a.activeTabRef()) != "alpha" {
		t.Fatalf("active tab = %+v", a.activeTabRef())
	}
}

func TestClosingBackgroundTabKeepsFrontTab(t *testing.T) {
	a := New()
	a.stop()
	a.terms = []*termSession{{ID: 1, Title: "alpha"}, {ID: 2, Title: "beta"}, {ID: 3, Title: "gamma"}}
	a.syncTabs()
	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	if err := tt.Click("gamma"); err != nil {
		t.Fatal(err)
	}
	if a.activeTab != 2 {
		t.Fatalf("activeTab = %d", a.activeTab)
	}
	// Closing a tab that is not in front must not move the front tab.
	if err := tt.Click("关闭 beta"); err != nil {
		t.Fatal(err)
	}
	if len(a.tabs) != 2 || a.activeTab != 1 || a.tabTitle(a.activeTabRef()) != "gamma" {
		t.Fatalf("tabs=%d active=%d title=%q", len(a.tabs), a.activeTab, a.tabTitle(a.activeTabRef()))
	}
}

func TestClosingLastTabShowsWelcome(t *testing.T) {
	a := New()
	a.stop()
	a.terms = []*termSession{{ID: 1, Title: "alpha"}}
	a.syncTabs()
	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	if err := tt.Click("关闭 alpha"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.tabs) != 0 || a.activeTerm != -1 {
		t.Fatalf("tabs=%d activeTerm=%d", len(a.tabs), a.activeTerm)
	}
	if !tt.HasText("还没有打开的会话") {
		t.Fatalf("welcome page missing; texts = %q", tt.Texts())
	}
}

func TestSameServerCanOpenMultipleTabs(t *testing.T) {
	a := New()
	a.stop()
	// Two shells on one saved server: distinct sessions, distinct tabs.
	a.terms = []*termSession{
		{ID: 1, Title: "测试", profileID: 7},
		{ID: 2, Title: "测试", profileID: 7},
	}
	a.syncTabs()
	if len(a.tabs) != 2 {
		t.Fatalf("tabs = %d, want 2", len(a.tabs))
	}
	if got := a.openSessionsForProfile(7); got != 2 {
		t.Fatalf("openSessionsForProfile = %d, want 2", got)
	}
	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	// Tabs of the same server are numbered so they can be told apart.
	for _, want := range []string{"测试 1", "测试 2"} {
		if !tt.HasText(want) {
			t.Fatalf("tab strip missing %q; texts = %q", want, tt.Texts())
		}
	}
	if err := tt.Click("关闭 测试 1"); err != nil {
		t.Fatal(err)
	}
	if len(a.tabs) != 1 || a.openSessionsForProfile(7) != 1 {
		t.Fatalf("tabs=%d open=%d, want 1/1", len(a.tabs), a.openSessionsForProfile(7))
	}
}

func TestMonitorHeaderShowsAndCopiesIP(t *testing.T) {
	a := New()
	a.stop()
	a.terms = []*termSession{{
		ID: 1, Title: "主机 A", profileID: 7,
		profile: sshx.Profile{Host: "10.1.2.3", Port: 22, Username: "root"},
	}}
	a.syncTabs()

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	if !tt.HasText("10.1.2.3") {
		t.Fatalf("monitor header missing the server IP; texts = %q", tt.Texts())
	}
	if tt.HasText("点击复制") {
		t.Fatalf("the IP row must not show a click hint; texts = %q", tt.Texts())
	}
	if err := tt.Click("复制 IP 地址 10.1.2.3"); err != nil {
		t.Fatal(err)
	}
	if got := tt.Clipboard(); got != "10.1.2.3" {
		t.Fatalf("clipboard = %q, want the server IP", got)
	}
	if !strings.Contains(a.status, "已复制 IP") {
		t.Fatalf("status = %q", a.status)
	}
	tt.Frame()
	if !tt.HasText("已复制") {
		t.Fatalf("copy toast missing; texts = %q", tt.Texts())
	}
}

func TestConnectSavedProfileAlwaysOpensNewTab(t *testing.T) {
	a := New()
	a.stop()
	p := quickTestProfile()
	a.profiles = []store.Profile{p}
	// The same server is already open in a tab: clicking it again must dial
	// a second session instead of focusing the first tab.
	a.terms = []*termSession{{ID: 1, Title: "alpha", profileID: p.ID}}
	a.syncTabs()
	a.connectSavedProfile(p)
	if !a.connecting || a.connectingProfileID != p.ID {
		t.Fatalf("connecting=%v profile=%d, want a new dial", a.connecting, a.connectingProfileID)
	}
	if a.activeTab != 0 {
		t.Fatalf("activeTab = %d, want the existing tab until the new one connects", a.activeTab)
	}
}

func TestPlusButtonStaysVisibleWithManyTabs(t *testing.T) {
	a := New()
	a.stop()
	for i := 0; i < 8; i++ {
		a.terms = append(a.terms, &termSession{ID: int64(i + 1), Title: "host-number-" + string(rune('a'+i))})
	}
	a.syncTabs()

	const winW = 1280
	tt := ui.NewTester(a.View, winW, 800)
	tt.Frame()
	box, ok := tt.Find("新建标签页")
	if !ok {
		t.Fatal("new-tab button has no label box")
	}
	if box.X+box.W > winW {
		t.Fatalf("new-tab button is off-screen: x=%.1f w=%.1f window=%d", box.X, box.W, winW)
	}
}

func TestPlusButtonOpensQuickTab(t *testing.T) {
	a := New()
	a.stop()
	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()

	if err := tt.Click("新建标签页"); err != nil {
		t.Fatal(err)
	}
	if len(a.tabs) != 1 || a.tabs[0].kind != tabQuickConnect {
		t.Fatalf("tabs = %+v", a.tabs)
	}
	if !a.quickTabActive() || a.activeTerm != -1 {
		t.Fatalf("quick tab not active: quick=%v activeTerm=%d", a.quickTabActive(), a.activeTerm)
	}
}
