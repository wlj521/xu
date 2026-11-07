package app

import (
	"context"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"xu/internal/store"
)

func TestGroupLookupHelpers(t *testing.T) {
	a := New()
	a.groups = []store.Group{{ID: 1, Name: "生产"}, {ID: 2, Name: "测试"}}

	if got := a.groupIDByName("生产"); got != 1 {
		t.Fatalf("groupIDByName = %d", got)
	}
	if got := a.groupIDByName(ungroupedName); got != 0 {
		t.Fatalf("ungrouped must map to 0, got %d", got)
	}
	if got := a.groupNameByID(2); got != "测试" {
		t.Fatalf("groupNameByID = %q", got)
	}
	if got := a.groupNameByID(99); got != ungroupedName {
		t.Fatalf("missing group must fall back to ungrouped, got %q", got)
	}

	a.groupChoice = "已经删除的分组"
	a.ensureGroupChoice()
	if a.groupChoice != ungroupedName {
		t.Fatalf("groupChoice = %q", a.groupChoice)
	}
}

func TestSidebarRendersUserGroupsAndNoSidebarSearch(t *testing.T) {
	a := New()
	a.groups = []store.Group{{ID: 1, Name: "生产环境"}}
	a.profiles = []store.Profile{
		{ID: 1, Name: "alpha", Host: "example.com", Port: 22, Username: "root", GroupID: 1},
		{ID: 2, Name: "beta", Host: "example.org", Port: 22, Username: "root"},
	}

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()

	for _, want := range []string{"生产环境", "alpha", "未分组", "beta", "本地终端"} {
		if !tt.HasText(want) {
			t.Fatalf("sidebar missing %q; texts = %q", want, tt.Texts())
		}
	}
	if tt.HasText("搜索主机或用户") {
		t.Fatalf("sidebar search must be removed; texts = %q", tt.Texts())
	}
}

func TestGroupFormCreatesUserGroup(t *testing.T) {
	mygo.App.SetPath(mygo.PathUserData, t.TempDir())
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })

	st, err := store.Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	a := New()
	a.SetStore(st)
	if len(a.groups) != 0 {
		t.Fatalf("groups must start empty, got %+v", a.groups)
	}

	a.openGroupForm(0, "我的服务器")
	a.submitGroupForm()
	if len(a.groups) != 1 || a.groups[0].Name != "我的服务器" {
		t.Fatalf("groups = %+v", a.groups)
	}
	if a.groupFormOpen {
		t.Fatal("group form must close after saving")
	}
}

func TestSearchPaletteFiltersSavedProfiles(t *testing.T) {
	a := New()
	a.profiles = []store.Profile{
		{ID: 1, Name: "prod-web-01", Host: "10.0.0.5", Username: "root"},
		{ID: 2, Name: "prod-db-01", Host: "10.0.0.6", Username: "deploy"},
	}

	if got := len(a.matchingProfiles()); got != 2 {
		t.Fatalf("all profiles = %d", got)
	}
	a.searchQuery = "deploy"
	matches := a.matchingProfiles()
	if len(matches) != 1 || matches[0].ID != 2 {
		t.Fatalf("search matches = %+v", matches)
	}
	a.searchQuery = "10.0.0.5"
	matches = a.matchingProfiles()
	if len(matches) != 1 || matches[0].ID != 1 {
		t.Fatalf("host search matches = %+v", matches)
	}
	a.searchQuery = "nothing-here"
	if len(a.matchingProfiles()) != 0 {
		t.Fatalf("expected no matches")
	}
}
