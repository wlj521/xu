package app

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"

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

	monitorCollapsed bool // left-hand server monitor collapsed to its rail
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

	// tabs is the top tab strip: open terminal sessions and quick-connect
	// pages, in the order the user opened them. terms stays the canonical
	// list of sessions; activeTerm is the index of the active tab's session
	// (-1 while a quick-connect page is in front).
	tabs            []*appTab
	activeTab       int
	nextTabID       int64
	pendingQuickTab int64 // quick-connect tab that hosts the connecting session

	// Connection manager dialog ("连接管理器").
	managerOpen   bool
	managerFilter string
	managerSel    int64
	managerErr    string

	// Quick-connect page filter.
	quickFilter string

	// Host key confirmation, asked from a background dial and answered by
	// the in-app modal instead of a native message box.
	hostKeyOpen        bool
	hostKeyHost        string
	hostKeyFingerprint string
	hostKeyAck         chan bool

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
	if c.Shortcut(ui.Cmd|ui.Alt, ui.KeyT) {
		a.openQuickTab()
		a.status = "从快速连接中选择服务器"
	}
	if c.Shortcut(ui.Cmd|ui.Alt, ui.KeyC) {
		a.openConnectionManager()
	}
	if c.Shortcut(ui.Cmd, ui.KeyB) {
		a.toggleMonitor()
	}
	if c.Shortcut(ui.Cmd|ui.Alt, ui.KeyB) {
		a.toggleMonitor()
	}

	ui.Column(c).Fill().Background(t.Background).Children(func() {
		a.titleBar(c, t, bar)
		ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
			if a.monitorCollapsed {
				a.inspectorRail(c, t)
			} else {
				a.shellInspector(c, t)
			}
			a.shellMain(c, t)
		})
		if a.formOpen {
			if !a.hostKeyOpen {
				a.connectModal(c, t)
			}
		}
		if a.managerOpen {
			a.connectionManagerModal(c, t)
		}
		if a.settingsOpen {
			a.settingsModal(c, t)
		}
		if a.searchOpen {
			a.searchModal(c, t)
		}
		if a.hostKeyOpen {
			a.hostKeyModal(c, t)
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
	if tab := a.activeTabRef(); tab != nil && tab.kind == tabQuickConnect {
		a.pendingQuickTab = tab.id
	} else {
		a.pendingQuickTab = 0
	}

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
			a.pendingQuickTab = 0
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
		a.attachSessionTab(ts)
		a.formOpen = false
		a.managerOpen = false
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
		a.hostKeyHost = ""
		a.hostKeyFingerprint = ""
		a.status = ""
	})
}

func (a *App) trustHost(host, fingerprint string) bool {
	a.trustMu.Lock()
	defer a.trustMu.Unlock()
	return a.trusted[host] == fingerprint
}

func (a *App) confirmHostKey(hk *sshx.HostKeyError) bool {
	return a.askHostKey(hk)
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
	a.managerOpen = false
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
	a.managerOpen = false
	// A saved server opens a new tab every time: several shells on one host
	// are useful (and FinalShell behaves this way), so an existing session
	// never steals the click.
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
	a.formOpen = false
	a.managerOpen = false
	a.pickerOpen = false
	a.batchOpen = false
	a.settingsOpen = false
	a.filesOpen = false
	a.monitorOpen = false
	a.forwardOpen = false
	a.directConnect = true
	a.connectingProfileID = p.ID
	if tab := a.activeTabRef(); tab != nil && tab.kind == tabQuickConnect {
		// The quick-connect page the user is looking at becomes the session
		// tab once the handshake succeeds.
		a.pendingQuickTab = tab.id
	} else {
		a.pendingQuickTab = 0
	}
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
	a.removeProfile(store.Profile{ID: a.profileID, Name: a.profileName})
}

// removeProfile deletes a saved connection from the store and the keychain.
// It is shared by the connection form and the connection manager dialog.
func (a *App) removeProfile(p store.Profile) {
	if p.ID == 0 {
		return
	}
	name := strings.TrimSpace(p.Name)
	if name == "" {
		name = fmt.Sprintf("%d", p.ID)
	}

	res, _ := mygo.Dialog.Message(mygo.MessageOptions{
		Type:               mygo.MessageWarning,
		Message:            "删除连接「" + name + "」？",
		Buttons:            []string{"取消", "删除"},
		DestructiveButtons: []int{1},
	})
	if res.Button != 1 {
		return
	}

	if a.store != nil {
		if err := a.store.DeleteProfile(a.ctx, p.ID); err != nil {
			a.managerErr = "删除失败: " + err.Error()
			a.status = a.managerErr
			return
		}
	}
	_ = vault.Delete(p.ID, "password")
	if a.profileID == p.ID {
		a.resetForm()
		a.formOpen = false
	}
	if a.managerSel == p.ID {
		a.managerSel = 0
	}
	a.refreshProfiles()
	a.managerErr = ""
	a.status = "已删除连接 " + name
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

// closeTerm closes the tab of the i-th session. Tabs close at once, like the
// tab bar of a browser: the terminal keeps running nothing once closed.
func (a *App) closeTerm(i int) {
	a.syncTabs()
	if i < 0 || i >= len(a.terms) {
		return
	}
	if idx := a.termTabIndex(a.terms[i]); idx >= 0 {
		a.closeTabIndex(idx)
		return
	}
	a.closeTermSession(a.terms[i])
}

// closeTermSession releases everything a session owns and drops it from
// a.terms. Callers keep the tab strip in step (closeTabIndex does).
func (a *App) closeTermSession(ts *termSession) {
	if ts == nil {
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
	if ts == a.activeTerminal() {
		a.filesOpen = false
		a.monitorOpen = false
		a.forwardOpen = false
		a.finding = false
		a.resetFind()
	}

	idx := a.termIndex(ts)
	ts.Close()
	if idx >= 0 {
		a.terms = append(a.terms[:idx], a.terms[idx+1:]...)
	}
	if len(a.terms) == 0 {
		a.activeTerm = -1
		a.activeTab = 0
		a.status = "没有打开的会话，点 + 快速连接"
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
			{Label: "新建标签页", Accelerator: "CmdOrCtrl+Alt+T", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
				win.Update(func() {
					a.openQuickTab()
					a.status = "从快速连接中选择服务器"
				})
			}},
			{Label: "连接管理器", Accelerator: "CmdOrCtrl+Alt+C", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
				win.Update(func() { a.openConnectionManager() })
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
				win.Update(func() { a.toggleMonitor() })
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
