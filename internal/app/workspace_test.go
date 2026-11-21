package app

import (
	"context"
	"strings"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"xu/internal/store"
)

func TestWorkspaceSnapshotKeepsServerTabOrder(t *testing.T) {
	a := New()
	a.tabs = []*appTab{
		{id: 1, kind: tabQuickConnect},
		{id: 2, kind: tabTerminal, term: &termSession{profileID: 22}},
		{id: 3, kind: tabTerminal, term: &termSession{profileID: 11}},
		{id: 4, kind: tabConnecting, conn: &connectingTab{profile: store.Profile{ID: 33}}},
	}
	a.activeTab = 2
	a.terminalSplit = &terminalSplit{
		firstID: 2, secondID: 3, layout: splitLayoutRows, ratio: 41,
	}

	got := a.workspaceSnapshot()
	want := []int64{22, 11, 33}
	if len(got.Tabs) != len(want) {
		t.Fatalf("workspace tabs = %v, want %v", got.Tabs, want)
	}
	for i := range want {
		if got.Tabs[i] != want[i] {
			t.Fatalf("workspace tabs = %v, want %v", got.Tabs, want)
		}
	}
	if got.Active != 1 {
		t.Fatalf("active tab = %d, want 1", got.Active)
	}
	if got.Split == nil {
		t.Fatal("split layout was not saved")
	}
	if got.Split.Layout != "rows" || got.Split.First != 0 || got.Split.Second != 1 || got.Split.Ratio != 41 {
		t.Fatalf("saved split = %+v", got.Split)
	}
}

func TestWorkspaceSaveLoadAndRestoreTabs(t *testing.T) {
	mygo.App.SetPath(mygo.PathUserData, t.TempDir())
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })

	ctx := context.Background()
	st, err := store.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	for _, p := range []*store.Profile{
		{Name: "one", Host: "one.example.com", Port: 22, Username: "root", AuthType: "key"},
		{Name: "two", Host: "two.example.com", Port: 22, Username: "root", AuthType: "key"},
	} {
		if err := st.SaveProfile(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	a := New()
	a.SetStore(st)
	a.tabs = []*appTab{
		{id: 1, kind: tabTerminal, term: &termSession{profileID: 2}},
		{id: 2, kind: tabTerminal, term: &termSession{profileID: 1}},
	}
	a.activeTab = 1
	a.terminalSplit = &terminalSplit{
		firstID: 1, secondID: 2, layout: splitLayoutRows, ratio: 38,
	}
	a.saveWorkspace()

	b := New()
	b.SetStore(st)
	if !b.workspaceLoaded || !b.workspaceRestorePending {
		t.Fatalf("workspace was not loaded: loaded=%v pending=%v", b.workspaceLoaded, b.workspaceRestorePending)
	}
	if len(b.workspace.Tabs) != 2 || b.workspace.Tabs[0] != 2 || b.workspace.Active != 1 {
		t.Fatalf("loaded workspace = %+v", b.workspace)
	}
	if b.workspace.Split == nil || b.workspace.Split.Layout != "rows" ||
		b.workspace.Split.First != 0 || b.workspace.Split.Second != 1 ||
		b.workspace.Split.Ratio != 38 {
		t.Fatalf("loaded split = %+v", b.workspace.Split)
	}

	b.restoreWorkspaceTabs()
	if len(b.tabs) != 2 {
		t.Fatalf("restored tabs = %d, want 2", len(b.tabs))
	}
	for i, wantID := range []int64{2, 1} {
		tab := b.tabs[i]
		if tab.kind != tabConnecting || tab.conn == nil || tab.conn.profile.ID != wantID {
			t.Fatalf("tab %d = %+v, want profile %d", i, tab, wantID)
		}
		if !tab.conn.restored {
			t.Fatalf("tab %d did not stay idle after restore", i)
		}
	}
	if b.activeTab != 1 || b.connecting || len(b.pendingConnectIDs) != 0 {
		t.Fatalf("restore started a connection: active=%d connecting=%v queue=%v",
			b.activeTab, b.connecting, b.pendingConnectIDs)
	}
	if !strings.Contains(b.status, "已恢复 2 个标签") {
		t.Fatalf("restore status = %q", b.status)
	}
	if b.terminalSplit == nil || b.terminalSplit.firstID != b.tabs[0].id ||
		b.terminalSplit.secondID != b.tabs[1].id || b.terminalSplit.layout != splitLayoutRows ||
		b.terminalSplit.ratio != 38 {
		t.Fatalf("restored split = %+v", b.terminalSplit)
	}
}

func TestWorkspaceIgnoresSplitNotInvolvingActiveTab(t *testing.T) {
	mygo.App.SetPath(mygo.PathUserData, t.TempDir())
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })

	ctx := context.Background()
	st, err := store.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.SetSetting(ctx, workspaceSettingKey,
		`{"tabs":[1,2,3],"active":2,"split":{"layout":"columns","first":0,"second":1,"ratio":50}}`); err != nil {
		t.Fatal(err)
	}

	a := New()
	a.SetStore(st)
	if a.workspace.Split != nil {
		t.Fatalf("split not containing the active tab was kept: %+v", a.workspace.Split)
	}
}

func TestWorkspaceRestoreSkipsDeletedProfiles(t *testing.T) {
	a := New()
	a.profiles = []store.Profile{{
		ID: 2, Name: "one", Host: "one.example.com", Port: 22, Username: "root", AuthType: "key",
	}}
	a.workspace = workspaceState{Tabs: []int64{999, 2}, Active: 1}

	a.restoreWorkspaceTabs()
	if len(a.tabs) != 1 || a.tabs[0].conn == nil || a.tabs[0].conn.profile.ID != 2 {
		t.Fatalf("restored tabs = %+v", a.tabs)
	}
	// The saved active tab no longer exists, so the first restored server wins.
	if a.activeTab != 0 || len(a.pendingConnectIDs) != 0 {
		t.Fatalf("restore state: active=%d queue=%v", a.activeTab, a.pendingConnectIDs)
	}
	if !a.tabs[0].conn.restored {
		t.Fatal("a restored tab did not stay idle")
	}
}

func TestWorkspaceRestoreKeepsTabsIdle(t *testing.T) {
	a := New()
	a.profiles = []store.Profile{
		{ID: 1, Name: "one", Host: "one.example.com", Port: 22, Username: "root", AuthType: "key", JumpProfileID: 99},
		{ID: 2, Name: "two", Host: "two.example.com", Port: 22, Username: "root", AuthType: "key", JumpProfileID: 99},
	}
	a.workspace = workspaceState{Tabs: []int64{1, 2}, Active: 0}

	a.restoreWorkspaceTabs()
	if a.connecting || len(a.pendingConnectIDs) != 0 {
		t.Fatalf("restore dialed eagerly: connecting=%v queue=%v", a.connecting, a.pendingConnectIDs)
	}
	if len(a.tabs) != 2 {
		t.Fatalf("restored tabs = %d, want 2", len(a.tabs))
	}
	for i, tab := range a.tabs {
		if tab.conn == nil || !tab.conn.restored || tab.conn.err != "" {
			t.Fatalf("tab %d = %+v, want an idle restored tab", i, tab.conn)
		}
	}
}

func TestRestoredWorkspaceTabShowsConnectPrompt(t *testing.T) {
	a := New()
	p := store.Profile{ID: 7, Name: "prod", Host: "prod.example.com", Port: 22, Username: "root"}
	a.tabs = []*appTab{{
		id: 1, kind: tabConnecting,
		conn: &connectingTab{profile: p, restored: true},
	}}
	a.activeTab = 0

	tt := ui.NewTester(a.View, 1280, 800)
	tt.Frame()
	for _, want := range []string{"已恢复", "尚未连接", "点击连接后才会创建 SSH、SFTP 与监控会话", "连接"} {
		if !tt.HasText(want) {
			t.Fatalf("restored tab does not show %q; texts = %q", want, tt.Texts())
		}
	}
	if tt.HasText("正在建立 TCP 连接") {
		t.Fatal("a restored tab started dialing at startup")
	}
}

func TestRestoredTabConnectsOnDemand(t *testing.T) {
	a := New()
	a.stop()
	p := store.Profile{
		ID: 7, Name: "prod", Host: "prod.example.com", Port: 22,
		Username: "root", AuthType: "key", JumpProfileID: 99,
	}
	a.profiles = []store.Profile{p}
	a.tabs = []*appTab{{
		id: 1, kind: tabConnecting,
		conn: &connectingTab{profile: p, restored: true},
	}}
	a.activeTab = 0

	a.connectTab(a.tabs[0])
	if a.tabs[0].conn == nil || a.tabs[0].conn.restored {
		t.Fatalf("restored tab did not leave its idle state: %+v", a.tabs[0].conn)
	}
	if a.tabs[0].conn.err == "" {
		t.Fatal("the invalid test connection did not fail fast")
	}
}

func TestWorkspaceRestoreCanBeDisabled(t *testing.T) {
	mygo.App.SetPath(mygo.PathUserData, t.TempDir())
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })

	ctx := context.Background()
	st, err := store.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.SetSetting(ctx, workspaceSettingKey, `{"tabs":[1],"active":0}`); err != nil {
		t.Fatal(err)
	}
	if err := st.SetSetting(ctx, "workspace.restore", "0"); err != nil {
		t.Fatal(err)
	}

	a := New()
	a.SetStore(st)
	if !a.workspaceLoaded || len(a.workspace.Tabs) != 1 {
		t.Fatalf("disabled workspace loaded tabs: %+v", a.workspace)
	}
	if a.workspaceRestorePending {
		t.Fatal("workspace restore stayed pending when disabled")
	}
}
