package app

import (
	"strings"

	"github.com/egoist/mygo"

	"xu/internal/store"
)

const ungroupedName = "未分组"

// refreshGroups reloads user-created groups. Groups are never seeded: an empty
// database means the sidebar only shows connections, until the user creates one.
func (a *App) refreshGroups() {
	if a.store == nil {
		return
	}
	groups, err := a.store.Groups(a.ctx)
	if err != nil {
		a.status = "读取分组失败: " + err.Error()
		return
	}
	a.groups = groups
	a.ensureGroupChoice()
}

func (a *App) groupOptions() []string {
	options := make([]string, 0, len(a.groups)+1)
	options = append(options, ungroupedName)
	for _, g := range a.groups {
		options = append(options, g.Name)
	}
	return options
}

func (a *App) groupIDByName(name string) int64 {
	name = strings.TrimSpace(name)
	if name == "" || name == ungroupedName {
		return 0
	}
	for _, g := range a.groups {
		if g.Name == name {
			return g.ID
		}
	}
	return 0
}

func (a *App) groupNameByID(id int64) string {
	if id == 0 {
		return ungroupedName
	}
	for _, g := range a.groups {
		if g.ID == id {
			return g.Name
		}
	}
	return ungroupedName
}

func (a *App) ensureGroupChoice() {
	if a.groupChoice == "" {
		a.groupChoice = ungroupedName
	}
	if a.groupChoice != ungroupedName && a.groupIDByName(a.groupChoice) == 0 {
		a.groupChoice = ungroupedName
	}
}

func (a *App) profilesInGroup(groupID int64) []store.Profile {
	out := make([]store.Profile, 0, len(a.profiles))
	for _, p := range a.profiles {
		if p.GroupID == groupID {
			out = append(out, p)
		}
	}
	return out
}

func (a *App) openGroupForm(id int64, name string) {
	a.groupFormOpen = true
	a.groupEditID = id
	a.groupName = name
	a.status = ""
}

func (a *App) cancelGroupForm() {
	a.groupFormOpen = false
	a.groupEditID = 0
	a.groupName = ""
}

func (a *App) submitGroupForm() {
	name := strings.TrimSpace(a.groupName)
	if name == "" {
		a.status = "分组名称不能为空"
		return
	}
	if name == ungroupedName {
		a.status = "「未分组」是系统保留名称"
		return
	}
	if a.store == nil {
		a.status = "会话库未就绪，暂时不能保存分组"
		return
	}
	g := store.Group{ID: a.groupEditID, Name: name}
	if err := a.store.SaveGroup(a.ctx, &g); err != nil {
		a.status = "保存分组失败: " + err.Error()
		return
	}
	editing := a.groupEditID != 0
	a.cancelGroupForm()
	a.refreshGroups()
	if editing {
		a.status = "已重命名分组为 " + name
	} else {
		a.status = "已创建分组 " + name
	}
}

func (a *App) deleteGroup(g store.Group) {
	res, _ := mygo.Dialog.Message(mygo.MessageOptions{
		Type:               mygo.MessageWarning,
		Message:            "删除分组「" + g.Name + "」？\n\n分组内的连接会回到「未分组」，连接本身不会被删除。",
		Buttons:            []string{"取消", "删除分组"},
		DestructiveButtons: []int{1},
	})
	if res.Button != 1 {
		return
	}
	if a.store == nil {
		return
	}
	if err := a.store.DeleteGroup(a.ctx, g.ID); err != nil {
		a.status = "删除分组失败: " + err.Error()
		return
	}
	a.refreshGroups()
	a.refreshProfiles()
	a.status = "已删除分组 " + g.Name
}
