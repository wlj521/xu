package app

import (
	"encoding/json"
	"fmt"
)

const workspaceSettingKey = "workspace.open_tabs"

type workspaceState struct {
	Tabs   []int64 `json:"tabs"`
	Active int     `json:"active"`
}

func (a *App) loadWorkspace() {
	a.workspaceLoaded = true
	a.workspaceRestorePending = false
	a.workspace = workspaceState{}
	if a.store == nil {
		return
	}
	values, err := a.store.Settings(a.ctx)
	if err != nil {
		a.status = "读取工作区失败: " + err.Error()
		return
	}
	raw := values[workspaceSettingKey]
	if raw == "" {
		return
	}
	var saved workspaceState
	if err := json.Unmarshal([]byte(raw), &saved); err != nil {
		a.status = "工作区记录无效，已忽略"
		return
	}

	clean := make([]int64, 0, len(saved.Tabs))
	active := -1
	for i, id := range saved.Tabs {
		if id <= 0 {
			continue
		}
		if i == saved.Active {
			active = len(clean)
		}
		clean = append(clean, id)
	}
	if len(clean) == 0 {
		return
	}
	if active < 0 || active >= len(clean) {
		active = 0
	}
	a.workspace = workspaceState{Tabs: clean, Active: active}
	a.workspaceRestorePending = a.settings.RestoreWorkspace
}

func (a *App) workspaceSnapshot() workspaceState {
	state := workspaceState{Tabs: make([]int64, 0, len(a.tabs)), Active: -1}
	for i, tab := range a.tabs {
		var profileID int64
		switch tab.kind {
		case tabTerminal:
			if tab.term != nil {
				profileID = tab.term.profileID
			}
		case tabConnecting:
			if tab.conn != nil {
				profileID = tab.conn.profile.ID
			}
		}
		if profileID == 0 {
			continue
		}
		if i == a.activeTab {
			state.Active = len(state.Tabs)
		}
		state.Tabs = append(state.Tabs, profileID)
	}
	if len(state.Tabs) > 0 && state.Active < 0 {
		state.Active = 0
	}
	return state
}

func (a *App) saveWorkspace() {
	if a.store == nil || a.workspaceRestoring {
		return
	}
	if a.workspaceRestorePending && len(a.tabs) == 0 {
		return
	}
	data, err := json.Marshal(a.workspaceSnapshot())
	if err != nil {
		a.status = "保存工作区失败: " + err.Error()
		return
	}
	if err := a.store.SetSetting(a.ctx, workspaceSettingKey, string(data)); err != nil {
		a.status = "保存工作区失败: " + err.Error()
	}
}

func (a *App) restoreWorkspaceIfPending() {
	if !a.workspaceRestorePending || !a.workspaceLoaded || !a.settings.RestoreWorkspace {
		return
	}
	a.workspaceRestorePending = false
	a.restoreWorkspaceTabs()
	a.startNextWorkspaceConnect()
}

// restoreWorkspaceTabs opens every saved server tab as a connecting tab. It
// does not dial; startNextWorkspaceConnect serialises the actual dials because
// the connecting page tracks one in-flight handshake.
func (a *App) restoreWorkspaceTabs() {
	if len(a.workspace.Tabs) == 0 {
		return
	}
	a.workspaceRestoring = true
	a.saveProfile = false
	var activeID int64
	for i, profileID := range a.workspace.Tabs {
		profile, ok := a.profileByID(profileID)
		if !ok {
			continue
		}
		tab := a.connectingTab(profile)
		if i == a.workspace.Active {
			activeID = tab.id
		}
		a.workspaceQueue = append(a.workspaceQueue, tab.id)
	}
	if activeID == 0 && len(a.workspaceQueue) > 0 {
		activeID = a.workspaceQueue[0]
	}
	for i, id := range a.workspaceQueue {
		if id != activeID {
			continue
		}
		copy(a.workspaceQueue[1:i+1], a.workspaceQueue[:i])
		a.workspaceQueue[0] = id
		break
	}
	if idx := a.tabIndexByID(activeID); idx >= 0 {
		a.activateTabIndex(idx)
	}
	a.workspaceRestoring = false
	if len(a.workspaceQueue) > 0 {
		a.status = fmt.Sprintf("正在恢复 %d 个会话…", len(a.workspaceQueue))
	}
}

func (a *App) startNextWorkspaceConnect() {
	if a.connecting || a.workspaceRestoring {
		return
	}
	for len(a.workspaceQueue) > 0 {
		id := a.workspaceQueue[0]
		a.workspaceQueue = a.workspaceQueue[1:]
		idx := a.tabIndexByID(id)
		if idx < 0 {
			continue
		}
		tab := a.tabs[idx]
		if tab.kind != tabConnecting || tab.conn == nil || tab.conn.cancel != nil {
			continue
		}
		profile := tab.conn.profile
		a.saveProfile = false
		a.startConnectToTab(profile, tab, 0)
		if a.connecting {
			return
		}
		// A setup error (for example a missing jump host) leaves no dial to
		// finish, so continue with the next restored tab here.
	}
}
