package app

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"xu/internal/sshx"
	"xu/internal/store"
	"xu/internal/vault"
)

type App struct {
	win  *mygo.Window
	ctx  context.Context
	stop context.CancelFunc

	store       *store.Store
	profiles    []store.Profile
	groups      []store.Group
	profileID   int64
	profileName string
	saveProfile bool

	groupFormOpen   bool
	groupName       string
	groupEditID     int64
	groupChoice     string
	collapsedGroups map[int64]bool

	searchOpen  bool
	searchQuery string

	sidebarVisible   bool // right-hand server inspector
	sidebarCollapsed bool // left-hand connection tree
	nav              navView
	formOpen         bool
	pickerOpen       bool
	directConnect    bool
	systemDark       bool

	settingsSection int
	connFilter      string
	authFilter      string
	connPageSel     int64

	host     string
	port     string
	username string
	password string

	connecting          bool
	connectingProfileID int64
	connectErr          string
	status              string

	terms      []*termSession
	activeTerm int

	trustMu sync.Mutex
	trusted map[string]string

	filesOpen    bool
	fileSplit    float32
	fileSplitPct float32
	browsers     map[int64]*fileBrowser
	fileErr      string
	editorOpen   bool
	editor       *editorSession

	monitorOpen bool
	monitors    map[int64]*monitorState

	forwardOpen   bool
	forwardKind   int
	forwardListen string
	forwardTarget string
	forwardErr    string
	forwardSave   bool
	forwardAuto   bool
	forwards      map[int64][]*forwardEntry
	savedForwards []store.ForwardRule

	batchOpen    bool
	batchChoices []batchChoice
	batchCmd     string
	batchErr     string
	batchRunning bool
	batchCancel  context.CancelFunc
	batchResults []batchResult
	batchState   ui.ListState
	batchSel     int
	batchSplit   float32

	finding     bool
	query       string
	lastSearch  string
	searchTotal int
	searchIndex int
	lastIndex   int
	searchInit  bool

	settingsOpen       bool
	settings           appSettings
	settingsFontSize   string
	settingsFontSizeF  float64
	settingsScrollback string
	settingsScrollPick string
	settingsErr        string
	settingsStatus     string

	inspectorTab  int
	bottomFiles   bool
	bottomFileTab int
	splitPercent  float32

	commandInput   string
	commandHistory []string

	promptOpen   bool
	promptTitle  string
	promptValue  string
	promptAction func(string)

	transfer       *transferState
	transferLog    []transferRecord
	nextTransferID int64
}

func New() *App {
	ctx, cancel := context.WithCancel(context.Background())
	return &App{
		ctx:                ctx,
		stop:               cancel,
		sidebarVisible:     true,
		sidebarCollapsed:   false,
		formOpen:           false,
		systemDark:         true,
		port:               "22",
		activeTerm:         -1,
		saveProfile:        true,
		trusted:            make(map[string]string),
		fileSplit:          560,
		fileSplitPct:       50,
		browsers:           make(map[int64]*fileBrowser),
		monitors:           make(map[int64]*monitorState),
		forwardListen:      "127.0.0.1:8080",
		forwardSave:        true,
		forwards:           make(map[int64][]*forwardEntry),
		batchSel:           -1,
		batchSplit:         220,
		searchInit:         true,
		settings:           defaultSettings(),
		settingsFontSize:   "13",
		settingsFontSizeF:  13,
		settingsScrollback: "10",
		settingsScrollPick: "10 MB",
		bottomFiles:        true,
		splitPercent:       64,
		groupChoice:        "未分组",
		collapsedGroups:    make(map[int64]bool),
	}
}

func (a *App) Attach(w *mygo.Window) { a.win = w }

func (a *App) activateTerm(i int) {
	if a.editorOpen {
		a.status = "请先保存或关闭编辑器"
		return
	}
	if i < 0 || i >= len(a.terms) {
		return
	}
	if a.finding {
		a.closeFind()
	}
	a.activeTerm = i
	a.nav = navWorkspace
	a.formOpen = false
	a.pickerOpen = false
	a.filesOpen = false
	a.monitorOpen = false
	a.forwardOpen = false
	a.batchOpen = false
	a.settingsOpen = false
}

func (a *App) SetStatus(msg string) { a.status = msg }

func (a *App) SetStore(st *store.Store) {
	a.store = st
	if st == nil {
		return
	}
	a.loadSettings()
	a.refreshGroups()
	a.refreshProfiles()

	hosts, err := st.KnownHosts(a.ctx)
	if err != nil {
		a.status = "读取主机密钥失败: " + err.Error()
		return
	}
	a.trustMu.Lock()
	for host, fp := range hosts {
		a.trusted[host] = fp
	}
	a.trustMu.Unlock()
}

func (a *App) post(fn func()) {
	if a.win == nil || a.ctx.Err() != nil {
		return
	}
	a.win.Update(fn)
}

func (a *App) Close() {
	a.stop()
	if a.batchCancel != nil {
		a.batchCancel()
		a.batchCancel = nil
	}
	for id := range a.monitors {
		a.stopMonitor(id)
	}
	a.closeAllForwards()
	a.discardEditor()
	for _, b := range a.browsers {
		b.Close()
	}
	a.browsers = nil
	for _, t := range a.terms {
		t.Close()
	}
	a.terms = nil
	if a.store != nil {
		a.store.Close()
		a.store = nil
	}
}

func (a *App) View(c *ui.Context) {
	t := a.applyTheme(c)
	settingsWasOpen := a.settingsOpen
	bar := c.TitleBar()

	if c.Shortcut(ui.Cmd, ui.KeyK) {
		if a.searchOpen {
			a.searchOpen = false
		} else if !a.formOpen && !a.settingsOpen {
			a.searchOpen = true
			a.searchQuery = ""
		}
	}
	if c.Shortcut(ui.Cmd, ui.KeyJ) {
		a.toggleDock()
	}
	if c.Shortcut(ui.Cmd, ui.KeyB) {
		a.sidebarVisible = !a.sidebarVisible
	}
	if c.Shortcut(ui.Cmd|ui.Alt, ui.KeyB) {
		a.sidebarCollapsed = !a.sidebarCollapsed
		if a.sidebarCollapsed {
			a.status = "左侧连接栏已收起"
		} else {
			a.status = "左侧连接栏已展开"
		}
	}

	ui.Column(c).Fill().Background(t.Background).Children(func() {
		a.titleBar(c, t, bar)
		ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
			if !a.sidebarCollapsed {
				a.shellSidebar(c, t)
			}
			a.shellMain(c, t)
			if a.sidebarVisible {
				a.shellInspector(c, t)
			} else {
				a.inspectorRail(c, t)
			}
		})
		if a.formOpen {
			a.connectModal(c, t)
		}
		if a.settingsOpen {
			a.settingsModal(c, t)
		}
		if a.searchOpen {
			a.searchModal(c, t)
		}
		if settingsWasOpen && !a.settingsOpen {
			a.saveSettings()
			if a.settingsErr != "" {
				a.status = a.settingsErr
			}
			a.settingsStatus = ""
		}
	})
}

func (a *App) terminal(c *ui.Context) {
	if a.activeTerm < 0 || a.activeTerm >= len(a.terms) {
		return
	}

	ts := a.terms[a.activeTerm]
	t := c.Theme()

	if ts.local && ts.exited {
		ui.Row(c).Padding(6, 12).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "本地终端已退出").TextColor(ui.RGB(245, 158, 11))
			if ui.Button(c, "新开本地终端").Clicked() {
				a.OpenLocalTerminal()
			}
		})
		terminalView(c, ts)
		return
	}

	if !a.finding && a.lastSearch != "" {
		if ts.Term != nil {
			ts.Term.EndSearch()
		}
		a.resetFind()
	}

	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyF) {
		a.openFind()
	}
	if a.finding && ts.Term != nil {
		a.findBar(c, ts)
	}

	if ts.exited {
		ui.Row(c).Padding(6, 12).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "连接已断开").TextColor(ui.RGB(220, 38, 38))
			if ui.Button(c, "重连").Disabled(ts.reconnecting).Clicked() {
				a.reconnect(ts)
			}
			if ts.reconnecting {
				ui.Text(c, "重连中…").TextColor(t.TextMuted)
			}
			if ts.reconnectErr != "" {
				ui.Text(c, ts.reconnectErr).TextColor(ui.RGB(220, 38, 38))
			}
		})
	}

	terminalView(c, ts)
}

func (a *App) formProfile() (sshx.Profile, string, error) {
	host := strings.TrimSpace(a.host)
	user := strings.TrimSpace(a.username)
	if host == "" {
		return sshx.Profile{}, "", fmt.Errorf("请填写主机地址")
	}
	if user == "" {
		return sshx.Profile{}, "", fmt.Errorf("请填写用户名")
	}
	port, err := strconv.Atoi(strings.TrimSpace(a.port))
	if err != nil || port <= 0 || port > 65535 {
		return sshx.Profile{}, "", fmt.Errorf("端口必须是 1–65535")
	}
	name := strings.TrimSpace(a.profileName)
	if name == "" {
		name = user + "@" + host
	}
	return sshx.Profile{Host: host, Port: port, Username: user}, name, nil
}

func (a *App) effectivePassword() string {
	if a.password != "" {
		return a.password
	}
	if a.store == nil || a.profileID == 0 {
		return ""
	}
	pw, err := vault.Get(a.profileID, "password")
	if err != nil {
		return ""
	}
	return pw
}

func (a *App) connect() {
	if a.connecting {
		return
	}
	p, _, err := a.formProfile()
	if err != nil {
		a.connectErr = err.Error()
		a.status = "连接失败: " + err.Error()
		if a.directConnect {
			a.formOpen = true
			a.directConnect = false
		}
		a.connectingProfileID = 0
		return
	}
	p.Password = a.effectivePassword()
	direct := a.directConnect
	a.directConnect = false

	a.connectErr = ""
	a.connecting = true
	a.status = "正在连接 " + p.Username + "@" + p.Host + "…"
	go a.doConnect(p, direct)
}

func (a *App) doConnect(p sshx.Profile, direct bool) {
	fail := func(msg string) {
		a.post(func() {
			a.connecting = false
			a.connectingProfileID = 0
			a.connectErr = msg
			a.status = "连接失败: " + shortLabel(msg, 60)
			if direct {
				a.formOpen = true
			}
		})
	}

	client, err := a.dialWithPrompt(p)
	if err != nil {
		fail(err.Error())
		return
	}

	ts, err := newTermSession(client, p, 100, 30, a.post, a.onTermSearch, a.termDisplay())
	if err != nil {
		client.Close()
		fail("打开终端失败: " + err.Error())
		return
	}

	a.post(func() {
		a.terms = append(a.terms, ts)
		a.activeTerm = len(a.terms) - 1
		a.nav = navWorkspace
		a.formOpen = false
		a.filesOpen = false
		a.monitorOpen = false
		a.forwardOpen = false
		a.batchOpen = false
		a.settingsOpen = false
		a.connecting = false
		a.connectingProfileID = 0
		a.finding = false
		a.resetFind()
		a.saveCurrentProfile(p)
		ts.profileID = a.profileID
		a.autoStartForwards(ts)
		a.startMonitorBackground(ts)
		a.inspectorTab = 0
		a.bottomFiles = true
		a.bottomFileTab = 0
		a.splitPercent = 64
		a.commandInput = ""
		a.password = ""
		a.connectErr = ""
		a.directConnect = false
		a.status = ""
	})
}

func (a *App) trustHost(host, fingerprint string) bool {
	a.trustMu.Lock()
	defer a.trustMu.Unlock()
	return a.trusted[host] == fingerprint
}

func (a *App) confirmHostKey(hk *sshx.HostKeyError) bool {
	if a.win == nil {
		return false
	}

	done := make(chan bool, 1)
	a.post(func() {
		res, err := mygo.Dialog.Message(mygo.MessageOptions{
			Type: mygo.MessageWarning,
			Message: fmt.Sprintf(
				"首次连接 %s\n\n主机密钥指纹：\n%s\n\n如果不是你预期的服务器，请取消。",
				hk.Host, hk.Fingerprint,
			),
			Buttons: []string{"取消", "信任并连接"},
		})
		done <- err == nil && res.Button == 1
	})

	select {
	case ok := <-done:
		return ok
	case <-a.ctx.Done():
		return false
	case <-time.After(2 * time.Minute):
		return false
	}
}

func (a *App) saveKnownHost(hk *sshx.HostKeyError) {
	if a.store == nil {
		return
	}
	host, portStr, err := net.SplitHostPort(hk.Host)
	if err != nil {
		host = hk.Host
		portStr = "22"
	}
	port, _ := strconv.Atoi(portStr)
	if err := a.store.SaveKnownHost(a.ctx, host, port, hk.Fingerprint); err != nil {
		a.status = "保存主机密钥失败: " + err.Error()
	}
}

func (a *App) saveCurrentProfile(p sshx.Profile) {
	if a.store == nil || !a.saveProfile {
		return
	}

	name := strings.TrimSpace(a.profileName)
	if name == "" {
		name = p.Username + "@" + p.Host
	}

	authType := "key"
	if p.Password != "" {
		authType = "password"
	}

	prof := store.Profile{
		ID:       a.profileID,
		Name:     name,
		Host:     p.Host,
		Port:     p.Port,
		Username: p.Username,
		AuthType: authType,
		GroupID:  a.groupIDByName(a.groupChoice),
	}
	if err := a.store.SaveProfile(a.ctx, &prof); err != nil {
		a.status = "保存会话失败: " + err.Error()
		return
	}

	a.profileID = prof.ID
	a.profileName = prof.Name
	if p.Password != "" {
		if err := vault.Set(prof.ID, "password", p.Password); err != nil {
			a.status = "会话已保存，但密码未写入系统钥匙串"
		}
	}
	a.refreshProfiles()
}

func (a *App) loadProfile(p store.Profile) {
	a.editProfile(p)
}

func (a *App) editProfile(p store.Profile) {
	if a.connecting {
		a.status = "正在连接，请稍候"
		return
	}
	if a.editorOpen {
		a.status = "请先保存或关闭编辑器"
		return
	}
	if a.finding {
		a.closeFind()
	}
	a.profileID = p.ID
	a.profileName = p.Name
	a.host = p.Host
	a.port = strconv.Itoa(p.Port)
	a.username = p.Username
	a.password = ""
	a.groupChoice = a.groupNameByID(p.GroupID)
	a.connectErr = ""
	a.status = ""
	a.saveProfile = true
	a.formOpen = true
	a.pickerOpen = false
	a.directConnect = false
	a.batchOpen = false
	a.settingsOpen = false
}

func (a *App) connectSavedProfile(p store.Profile) {
	if a.connecting {
		a.status = "正在连接，请稍候"
		return
	}
	for i, ts := range a.terms {
		if !ts.local && !ts.exited && ts.profileID == p.ID {
			a.activateTerm(i)
			return
		}
	}
	if a.editorOpen {
		a.status = "请先保存或关闭编辑器"
		return
	}
	if a.finding {
		a.closeFind()
	}

	a.profileID = p.ID
	a.profileName = p.Name
	a.host = p.Host
	a.port = strconv.Itoa(p.Port)
	a.username = p.Username
	a.password = ""
	a.groupChoice = a.groupNameByID(p.GroupID)
	a.saveProfile = true
	a.connectErr = ""
	a.status = ""
	a.nav = navWorkspace
	a.formOpen = false
	a.pickerOpen = false
	a.batchOpen = false
	a.settingsOpen = false
	a.filesOpen = false
	a.monitorOpen = false
	a.forwardOpen = false
	a.directConnect = true
	a.connectingProfileID = p.ID
	a.connect()
}

func (a *App) saveProfileForm() {
	p, name, err := a.formProfile()
	if err != nil {
		a.connectErr = err.Error()
		return
	}
	if a.store == nil {
		a.connectErr = "会话库未就绪，暂时不能保存连接"
		return
	}

	authType := "key"
	if a.password != "" {
		authType = "password"
	} else if a.profileID != 0 {
		for _, old := range a.profiles {
			if old.ID == a.profileID {
				authType = old.AuthType
				break
			}
		}
	}

	prof := store.Profile{
		ID:       a.profileID,
		Name:     name,
		Host:     p.Host,
		Port:     p.Port,
		Username: p.Username,
		AuthType: authType,
		GroupID:  a.groupIDByName(a.groupChoice),
	}
	if err := a.store.SaveProfile(a.ctx, &prof); err != nil {
		a.connectErr = "保存失败: " + err.Error()
		return
	}
	a.profileID = prof.ID
	a.profileName = prof.Name
	if a.password != "" {
		if err := vault.Set(prof.ID, "password", a.password); err != nil {
			a.status = "已保存，但密码未写入系统钥匙串"
		}
	}
	a.refreshProfiles()
	a.connectErr = ""
	a.formOpen = false
	a.status = "已保存连接 " + prof.Name
}

func (a *App) deleteProfile() {
	if a.profileID == 0 {
		return
	}

	res, _ := mygo.Dialog.Message(mygo.MessageOptions{
		Type:               mygo.MessageWarning,
		Message:            "删除会话「" + a.profileName + "」？",
		Buttons:            []string{"取消", "删除"},
		DestructiveButtons: []int{1},
	})
	if res.Button != 1 {
		return
	}

	if a.store != nil {
		if err := a.store.DeleteProfile(a.ctx, a.profileID); err != nil {
			a.status = "删除失败: " + err.Error()
			return
		}
	}
	_ = vault.Delete(a.profileID, "password")
	a.resetForm()
	a.refreshProfiles()
	a.formOpen = false
	a.status = "已删除连接"
}

func (a *App) resetForm() {
	a.profileID = 0
	a.profileName = ""
	a.host = ""
	a.port = "22"
	a.username = ""
	a.password = ""
	a.connectErr = ""
	a.groupChoice = "未分组"
}

func (a *App) refreshProfiles() {
	if a.store == nil {
		return
	}
	profs, err := a.store.Profiles(a.ctx)
	if err != nil {
		a.status = "读取会话失败: " + err.Error()
		return
	}
	a.profiles = profs
}

func (a *App) closeTerm(i int) {
	if i < 0 || i >= len(a.terms) {
		return
	}
	ts := a.terms[i]

	res, _ := mygo.Dialog.Message(mygo.MessageOptions{
		Type:               mygo.MessageWarning,
		Message:            "关闭终端「" + ts.Title + "」？",
		Buttons:            []string{"取消", "关闭"},
		DestructiveButtons: []int{1},
	})
	if res.Button != 1 {
		return
	}

	if a.editor != nil && a.editor.termID == ts.ID {
		a.discardEditor()
	}
	a.stopMonitor(ts.ID)
	a.stopForwards(ts.ID)
	if b := a.browsers[ts.ID]; b != nil {
		b.Close()
		delete(a.browsers, ts.ID)
	}
	if i == a.activeTerm {
		a.filesOpen = false
		a.monitorOpen = false
		a.forwardOpen = false
		a.finding = false
		a.resetFind()
	}

	ts.Close()
	a.terms = append(a.terms[:i], a.terms[i+1:]...)
	if a.activeTerm >= len(a.terms) {
		a.activeTerm = len(a.terms) - 1
	}
	if len(a.terms) == 0 {
		a.formOpen = false
		a.status = "会话已关闭，可从左侧栏重新打开"
	}
}

func (a *App) Menu() *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: "文件", Submenu: []*mygo.MenuItem{
			{Label: "新建连接", Accelerator: "CmdOrCtrl+N", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
				win.Update(func() { a.openConnectForm() })
			}},
			{Label: "本地终端", Accelerator: "CmdOrCtrl+T", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
				win.Update(func() { a.OpenLocalTerminal() })
			}},
			mygo.Separator(),
			{Label: "文件面板", Accelerator: "CmdOrCtrl+Shift+O", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
				win.Update(func() { a.openFiles() })
			}},
			{Label: "底部面板", Accelerator: "CmdOrCtrl+J", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
				win.Update(func() { a.toggleDock() })
			}},
			mygo.Separator(),
			{Role: mygo.RoleQuit},
		}},
		{Label: "编辑", Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleUndo, Label: "撤销"},
			{Role: mygo.RoleRedo, Label: "重做"},
			mygo.Separator(),
			{Role: mygo.RoleCut, Label: "剪切"},
			{Role: mygo.RoleCopy, Label: "复制"},
			{Role: mygo.RolePaste, Label: "粘贴"},
			{Role: mygo.RoleSelectAll, Label: "全选"},
		}},
		{Label: "视图", Submenu: []*mygo.MenuItem{
			{Label: "查找", Accelerator: "CmdOrCtrl+Shift+F", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
				win.Update(func() { a.openFind() })
			}},
			{Label: "设置", Accelerator: "CmdOrCtrl+Alt+S", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
				win.Update(func() { a.openSettings() })
			}},
			{Label: "切换服务器监控", Accelerator: "CmdOrCtrl+B", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
				win.Update(func() { a.sidebarVisible = !a.sidebarVisible })
			}},
			{Label: "服务器监控", Accelerator: "CmdOrCtrl+Shift+M", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
				win.Update(func() { a.openMonitor() })
			}},
			{Label: "端口转发", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
				win.Update(func() { a.openForwards() })
			}},
			{Label: "批量命令", Accelerator: "CmdOrCtrl+Shift+B", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
				win.Update(func() { a.openBatch() })
			}},
			{Label: "打开数据目录", Click: func(_ *mygo.MenuItem, _ *mygo.Window) {
				if dir, err := mygo.App.Path(mygo.PathUserData); err == nil {
					_ = mygo.Shell.OpenPath(dir)
				}
			}},
			mygo.Separator(),
			{Role: mygo.RoleToggleFullScreen},
		}},
	})
}
