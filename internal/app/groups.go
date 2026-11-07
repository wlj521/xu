package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

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

func (a *App) groupFormRow(c *ui.Context, t *ui.Theme) {
	if !a.groupFormOpen {
		return
	}
	ui.Row(c).Shrink(0).Padding(0, 12, 8, 12).Gap(5).AlignItems(ui.Center).Children(func() {
		input := ui.TextInput(c, &a.groupName).Grow(1).Height(28).FontSize(10.5).
			Placeholder("分组名称").AutoFocus()
		if input.Submitted() {
			a.submitGroupForm()
		}
		if sidebarIconButton(c, t, "group-form-save", iconCheck, "保存分组").Clicked() {
			a.submitGroupForm()
		}
		if sidebarIconButton(c, t, "group-form-cancel", iconClose, "取消").Clicked() {
			a.cancelGroupForm()
		}
	})
}

func (a *App) groupHeaderRow(c *ui.Context, t *ui.Theme, id int64, name string, count int, editable bool) {
	collapsed := a.collapsedGroups[id]
	ui.Row(c).FillWidth().Height(28).Gap(1).AlignItems(ui.Center).Children(func() {
		head := ui.ButtonBase(c.Key(fmt.Sprintf("group-head-%d", id))).
			Grow(1).Height(28).Padding(0, 7).Radius(6).Gap(6).AlignItems(ui.Center)
		if head.Hovered() {
			head.Background(colPanel2)
		}
		head.Children(func() {
			chevron := iconChevron
			if collapsed {
				chevron = iconChevronRight
			}
			iconEl(c, chevron, 12, colMuted2)
			ui.Text(c, name).FontSize(10).FontWeight(600).LetterSpacing(0.2).
				TextColor(colMuted).NoWrap()
			ui.Text(c, "").Grow(1)
			if count >= 0 {
				ui.Text(c, fmt.Sprintf("%d", count)).FontSize(8).Font("Cascadia Code").
					TextColor(colMuted2).NoWrap()
			}
		})
		if head.Clicked() {
			a.collapsedGroups[id] = !collapsed
		}
		if editable {
			head.ContextMenu(func(m *ui.Menu) {
				if m.Item("重命名分组").Chosen() {
					a.openGroupForm(id, name)
				}
				if m.Item("删除分组").Chosen() {
					a.deleteGroup(store.Group{ID: id, Name: name})
				}
			})
		}
	})
}

func (a *App) groupBodyRow(c *ui.Context, t *ui.Theme, id int64, profiles []store.Profile) {
	if a.collapsedGroups[id] {
		return
	}
	if len(profiles) == 0 {
		ui.Text(c, "暂无连接").FontSize(9).TextColor(colMuted2).Padding(4, 16, 6, 22)
		return
	}
	for _, p := range profiles {
		p := p
		a.profileRow(c, t, p, a.profileConnected(p.ID))
	}
}
