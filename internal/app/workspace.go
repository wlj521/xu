package app

import (
	"encoding/json"
	"fmt"
)

const workspaceSettingKey = "workspace.open_tabs"

type workspaceState struct {
	Tabs   []int64         `json:"tabs"`
	Active int             `json:"active"`
	Split  *workspaceSplit `json:"split,omitempty"`
}

// workspaceSplit points at entries in Tabs, not saved connections: two tabs
// of the same server are still distinct panes when the workspace returns.
type workspaceSplit struct {
	Layout string  `json:"layout"`
	First  int     `json:"first"`
	Second int     `json:"second"`
	Ratio  float32 `json:"ratio,omitempty"`
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
	cleanIndex := make(map[int]int, len(saved.Tabs))
	active := -1
	for i, id := range saved.Tabs {
		if id <= 0 {
			continue
		}
		if i == saved.Active {
			active = len(clean)
		}
		cleanIndex[i] = len(clean)
		clean = append(clean, id)
	}
	if len(clean) == 0 {
		return
	}
	if active < 0 || active >= len(clean) {
		active = 0
	}

	var split *workspaceSplit
	if saved.Split != nil {
		first, firstOK := cleanIndex[saved.Split.First]
		second, secondOK := cleanIndex[saved.Split.Second]
		layout, layoutOK := splitLayoutFromWorkspace(saved.Split.Layout)
		if firstOK && secondOK && first != second && layoutOK &&
			(active == first || active == second) {
			ratio := saved.Split.Ratio
			if ratio < 10 || ratio > 90 {
				ratio = 50
			}
			split = &workspaceSplit{
				Layout: layout.workspaceName(),
				First:  first,
				Second: second,
				Ratio:  ratio,
			}
		}
	}
	a.workspace = workspaceState{Tabs: clean, Active: active, Split: split}
	a.workspaceRestorePending = a.settings.RestoreWorkspace
}

func (a *App) workspaceSnapshot() workspaceState {
	state := workspaceState{Tabs: make([]int64, 0, len(a.tabs)), Active: -1}
	tabIndex := make(map[int64]int, len(a.tabs))
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
		tabIndex[tab.id] = len(state.Tabs)
		state.Tabs = append(state.Tabs, profileID)
	}
	if len(state.Tabs) > 0 && state.Active < 0 {
		state.Active = 0
	}
	if s := a.terminalSplit; s != nil && a.splitActive() {
		first, firstOK := tabIndex[s.firstID]
		second, secondOK := tabIndex[s.secondID]
		if firstOK && secondOK && first != second &&
			(state.Active == first || state.Active == second) {
			state.Split = &workspaceSplit{
				Layout: s.layout.workspaceName(),
				First:  first,
				Second: second,
				Ratio:  s.ratio,
			}
		}
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
		return
	}
	a.splitRatioDirty = false
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
	tabIDByIndex := make(map[int]int64, len(a.workspace.Tabs))
	for i, profileID := range a.workspace.Tabs {
		profile, ok := a.profileByID(profileID)
		if !ok {
			continue
		}
		tab := a.connectingTab(profile)
		tabIDByIndex[i] = tab.id
		if i == a.workspace.Active {
			activeID = tab.id
		}
		a.workspaceQueue = append(a.workspaceQueue, tab.id)
	}
	a.terminalSplit = nil
	if split := a.workspace.Split; split != nil {
		first, firstOK := tabIDByIndex[split.First]
		second, secondOK := tabIDByIndex[split.Second]
		if firstOK && secondOK && first != second {
			layout, _ := splitLayoutFromWorkspace(split.Layout)
			ratio := split.Ratio
			if ratio < 10 || ratio > 90 {
				ratio = 50
			}
			a.terminalSplit = &terminalSplit{
				firstID:  first,
				secondID: second,
				layout:   layout,
				ratio:    ratio,
			}
		}
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
