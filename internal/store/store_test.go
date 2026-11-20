package store

import (
	"context"
	"testing"
	"time"

	"github.com/egoist/mygo"
)

func TestProfilesAndKnownHosts(t *testing.T) {
	mygo.App.SetPath(mygo.PathUserData, t.TempDir())
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })

	ctx := context.Background()
	st, err := Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	p := Profile{Name: "test", Host: "127.0.0.1", Port: 22, Username: "root", AuthType: "password"}
	if err := st.SaveProfile(ctx, &p); err != nil {
		t.Fatal(err)
	}
	if p.ID == 0 {
		t.Fatal("expected an inserted ID")
	}

	list, err := st.Profiles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "test" || list[0].Port != 22 {
		t.Fatalf("profiles = %+v", list)
	}

	g := Group{Name: "生产环境"}
	if err := st.SaveGroup(ctx, &g); err != nil {
		t.Fatal(err)
	}
	if g.ID == 0 {
		t.Fatal("expected a group ID")
	}
	p.GroupID = g.ID
	if err := st.SaveProfile(ctx, &p); err != nil {
		t.Fatal(err)
	}
	list, err = st.Profiles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].GroupID != g.ID {
		t.Fatalf("profile group = %+v", list)
	}
	groups, err := st.Groups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].Name != "生产环境" {
		t.Fatalf("groups = %+v", groups)
	}
	g.Name = "线上"
	if err := st.SaveGroup(ctx, &g); err != nil {
		t.Fatal(err)
	}
	groups, err = st.Groups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].Name != "线上" {
		t.Fatalf("renamed groups = %+v", groups)
	}
	if err := st.DeleteGroup(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	list, err = st.Profiles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].GroupID != 0 {
		t.Fatalf("profile after group delete = %+v", list)
	}
	groups, err = st.Groups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Fatalf("groups after delete = %+v", groups)
	}

	if err := st.SaveKnownHost(ctx, "127.0.0.1", 22, "SHA256:test"); err != nil {
		t.Fatal(err)
	}
	hosts, err := st.KnownHosts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if hosts["127.0.0.1:22"] != "SHA256:test" {
		t.Fatalf("hosts = %+v", hosts)
	}

	rule := ForwardRule{
		ProfileID: p.ID,
		Kind:      0,
		Listen:    "127.0.0.1:8080",
		Target:    "127.0.0.1:80",
		AutoStart: true,
	}
	if err := st.SaveForward(ctx, &rule); err != nil {
		t.Fatal(err)
	}
	if rule.ID == 0 {
		t.Fatal("expected a forward ID")
	}
	rules, err := st.Forwards(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || !rules[0].AutoStart {
		t.Fatalf("forwards = %+v", rules)
	}

	// The same rule saves once: it is replaced, not duplicated.
	dup := ForwardRule{
		ProfileID: p.ID,
		Kind:      0,
		Listen:    "127.0.0.1:8080",
		Target:    "127.0.0.1:80",
	}
	if err := st.SaveForward(ctx, &dup); err != nil {
		t.Fatal(err)
	}
	rules, err = st.Forwards(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0].AutoStart {
		t.Fatalf("forwards after replace = %+v", rules)
	}

	if err := st.DeleteForward(ctx, dup.ID); err != nil {
		t.Fatal(err)
	}
	rules, err = st.Forwards(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 0 {
		t.Fatalf("forwards after delete = %+v", rules)
	}

	if err := st.SaveForward(ctx, &ForwardRule{
		ProfileID: p.ID,
		Kind:      2,
		Listen:    "127.0.0.1:1080",
		AutoStart: true,
	}); err != nil {
		t.Fatal(err)
	}

	if err := st.SetSetting(ctx, "terminal.font.size", "16"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetSetting(ctx, "terminal.font.size", "18"); err != nil {
		t.Fatal(err)
	}
	settings, err := st.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings["terminal.font.size"] != "18" {
		t.Fatalf("settings = %+v", settings)
	}

	if err := st.DeleteProfile(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	list, err = st.Profiles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("profiles after delete = %+v", list)
	}
	rules, err = st.Forwards(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 0 {
		t.Fatalf("forwards after profile delete = %+v", rules)
	}
}

func TestQuickCommandsCRUD(t *testing.T) {
	mygo.App.SetPath(mygo.PathUserData, t.TempDir())
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })

	ctx := context.Background()
	st, err := Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	p := Profile{Name: "jt808", Host: "127.0.0.1", Port: 22, Username: "root"}
	if err := st.SaveProfile(ctx, &p); err != nil {
		t.Fatal(err)
	}

	global := QuickCommand{Name: "uptime", Command: "uptime"}
	server := QuickCommand{
		ProfileID: p.ID,
		Name:      "进入项目",
		Command:   "cd /projects/jt808",
	}
	for _, q := range []*QuickCommand{&global, &server} {
		if err := st.SaveQuickCommand(ctx, q); err != nil {
			t.Fatal(err)
		}
		if q.ID == 0 {
			t.Fatalf("quick command %q got no ID", q.Name)
		}
	}

	list, err := st.QuickCommands(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ProfileID != 0 || list[1].ProfileID != p.ID {
		t.Fatalf("quick commands = %+v", list)
	}

	global.Command = "uptime -p"
	if err := st.SaveQuickCommand(ctx, &global); err != nil {
		t.Fatal(err)
	}
	list, err = st.QuickCommands(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Command != "uptime -p" {
		t.Fatalf("updated quick commands = %+v", list)
	}

	if err := st.DeleteQuickCommand(ctx, global.ID); err != nil {
		t.Fatal(err)
	}
	list, err = st.QuickCommands(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "进入项目" {
		t.Fatalf("quick commands after delete = %+v", list)
	}

	if err := st.DeleteProfile(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	list, err = st.QuickCommands(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("quick commands after profile delete = %+v", list)
	}
}

func TestJumpProfilePersistsAndClearsOnDelete(t *testing.T) {
	mygo.App.SetPath(mygo.PathUserData, t.TempDir())
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })

	ctx := context.Background()
	st, err := Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	jump := Profile{Name: "bastion", Host: "10.0.0.1", Port: 22, Username: "ops", AuthType: "agent"}
	if err := st.SaveProfile(ctx, &jump); err != nil {
		t.Fatal(err)
	}
	target := Profile{
		Name:          "private-db",
		Host:          "10.1.0.8",
		Port:          22,
		Username:      "deploy",
		AuthType:      "key",
		JumpProfileID: jump.ID,
	}
	if err := st.SaveProfile(ctx, &target); err != nil {
		t.Fatal(err)
	}

	list, err := st.Profiles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("profiles = %+v", list)
	}
	var got Profile
	for _, p := range list {
		if p.ID == target.ID {
			got = p
		}
	}
	if got.JumpProfileID != jump.ID {
		t.Fatalf("jump profile = %d, want %d", got.JumpProfileID, jump.ID)
	}

	if err := st.DeleteProfile(ctx, jump.ID); err != nil {
		t.Fatal(err)
	}
	list, err = st.Profiles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].JumpProfileID != 0 {
		t.Fatalf("profile after jump delete = %+v", list)
	}
}

func TestTransferHistoryPersists(t *testing.T) {
	mygo.App.SetPath(mygo.PathUserData, t.TempDir())
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })

	ctx := context.Background()
	st, err := Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	for _, rec := range []TransferRecord{
		{Kind: "upload", Label: "上传 a.txt", Target: "/tmp/a.txt", Done: 10, Total: 10, Status: "done"},
		{Kind: "download", Label: "下载 b.log", Target: "C:\\tmp\\b.log", Done: 4, Total: 8, Status: "failed", Err: "network"},
	} {
		rec := rec
		if err := st.AddTransferRecord(ctx, &rec); err != nil {
			t.Fatal(err)
		}
		if rec.ID == 0 {
			t.Fatal("transfer record got no ID")
		}
	}
	old := TransferRecord{
		Kind: "upload", Label: "过期记录", Status: "done",
		CreatedAt: time.Now().Add(-8 * 24 * time.Hour).Unix(),
	}
	if err := st.AddTransferRecord(ctx, &old); err != nil {
		t.Fatal(err)
	}

	history, err := st.TransferHistory(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[0].Status != "failed" || history[1].Label != "上传 a.txt" {
		t.Fatalf("history = %+v", history)
	}

	if err := st.ClearTransferHistory(ctx); err != nil {
		t.Fatal(err)
	}
	history, err = st.TransferHistory(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 0 {
		t.Fatalf("history after clear = %+v", history)
	}
}
