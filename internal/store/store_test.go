package store

import (
	"context"
	"testing"

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
