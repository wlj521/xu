package app

import (
	"context"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"xu/internal/store"
)

func TestKnownHostsManagementListsAndDeletes(t *testing.T) {
	mygo.App.SetPath(mygo.PathUserData, t.TempDir())
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })

	ctx := context.Background()
	st, err := store.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveKnownHost(ctx, "db.example.com", 2222, hostKeyTestFingerprint); err != nil {
		t.Fatal(err)
	}

	a := New()
	a.SetStore(st)
	defer a.Close()
	if got := a.knownHostFingerprint("db.example.com:2222"); got != hostKeyTestFingerprint {
		t.Fatalf("trusted fingerprint = %q", got)
	}

	a.openKnownHosts()
	tt := ui.NewTester(func(c *ui.Context) {
		theme := a.applyTheme(c)
		a.knownHostsModal(c, theme)
	}, 1024, 720)
	tt.Frame()
	for _, want := range []string{
		"已知主机",
		"已保存 1 个 SSH 主机密钥",
		"db.example.com:2222",
		hostKeyTestFingerprint,
		"删除",
	} {
		if !tt.HasText(want) {
			t.Fatalf("known hosts dialog does not show %q; texts = %q", want, tt.Texts())
		}
	}

	if err := tt.Click("删除"); err != nil {
		t.Fatalf("delete known host: %v", err)
	}
	tt.Frame()

	records, err := st.KnownHostRecords(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatalf("known hosts after delete = %+v", records)
	}
	if got := a.knownHostFingerprint("db.example.com:2222"); got != "" {
		t.Fatalf("deleted host is still trusted with %q", got)
	}
	if !a.knownHostsOpen {
		t.Fatal("deleting an entry closed the management dialog")
	}
	if !tt.HasText("还没有保存的主机密钥") {
		t.Fatalf("the empty state is missing; texts = %q", tt.Texts())
	}
}

func TestKnownHostsSettingsShowsManagerEntry(t *testing.T) {
	mygo.App.SetPath(mygo.PathUserData, t.TempDir())
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })

	ctx := context.Background()
	st, err := store.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveKnownHost(ctx, "db.example.com", 2222, hostKeyTestFingerprint); err != nil {
		t.Fatal(err)
	}

	a := New()
	a.SetStore(st)
	defer a.Close()
	a.formOpen = false
	a.openSettings()
	a.settingsSection = 3

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	for _, want := range []string{"主机密钥", "管理"} {
		if !tt.HasText(want) {
			t.Fatalf("connection settings does not show %q; texts = %q", want, tt.Texts())
		}
	}

	if err := tt.Click("管理"); err != nil {
		t.Fatalf("open known hosts manager: %v", err)
	}
	tt.Frame()
	if !a.knownHostsOpen {
		t.Fatal("clicking 管理 did not open the known hosts dialog")
	}
	for _, want := range []string{"已知主机", "db.example.com:2222", hostKeyTestFingerprint} {
		if !tt.HasText(want) {
			t.Fatalf("known hosts dialog does not show %q; texts = %q", want, tt.Texts())
		}
	}

	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.knownHostsOpen {
		t.Fatal("Escape did not close the known hosts dialog")
	}
	if !a.settingsOpen {
		t.Fatal("closing the known hosts dialog also closed settings")
	}
}
