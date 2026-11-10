package app

import (
	"context"
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"

	"xu/internal/sshx"
	"xu/internal/store"
)

// connectingTab is what a tab holds while its SSH session is being
// established: the target, the last failure, and how to cancel the dial.
type connectingTab struct {
	profile store.Profile
	err     string
	cancel  context.CancelFunc
	// origin is the quick-connect page this dial was started from. It stays
	// open while the dial runs and closes when the connection succeeds, the
	// click having consumed it.
	origin int64
}

func profileDisplayName(p store.Profile) string {
	if p.Name != "" {
		return p.Name
	}
	return p.Username + "@" + p.Host
}

// connectingTabActive reports whether the tab in front is still connecting.
func (a *App) connectingTabActive() bool {
	tab := a.activeTabRef()
	return tab != nil && tab.kind == tabConnecting
}

// activeConnectingProfile is the server the connecting tab in front dials,
// so the side monitor can name it before the session exists.
func (a *App) activeConnectingProfile() (store.Profile, bool) {
	tab := a.activeTabRef()
	if tab == nil || tab.kind != tabConnecting || tab.conn == nil {
		return store.Profile{}, false
	}
	return tab.conn.profile, true
}

// connectingTab opens a new tab in front at once, so the click visibly opens
// a tab before the handshake starts. The quick-connect page that started it
// stays open while the dial runs and closes when the session appears; the
// session replaces the tab's contents in place.
func (a *App) connectingTab(p store.Profile) *appTab {
	tab := &appTab{id: a.newTabID(), kind: tabConnecting, conn: &connectingTab{profile: p}}
	a.tabs = append(a.tabs, tab)
	a.activeTab = len(a.tabs) - 1
	a.activeTerm = -1
	return tab
}

// startConnectToTab starts the dial that fills tab; the tab shows the real
// phase and elapsed time until the session replaces it. origin is the
// quick-connect page to close once the connection succeeds.
func (a *App) startConnectToTab(p store.Profile, tab *appTab, origin int64) {
	if tab == nil {
		return
	}

	a.applyProfile(p)
	a.connecting = true
	a.connectingProfileID = p.ID
	a.pendingQuickTab = 0
	a.connectErr = ""
	a.status = "正在连接 " + profileDisplayName(p) + "…"

	sp := sshx.Profile{
		Host:     p.Host,
		Port:     p.Port,
		Username: p.Username,
		Password: a.effectivePassword(),
	}
	ctx, cancel := context.WithCancel(a.ctx)
	tab.kind = tabConnecting
	tab.term = nil
	tab.conn = &connectingTab{profile: p, cancel: cancel, origin: origin}

	a.startConnectTimer(tab.id)
	go a.doConnect(ctx, cancel, sp, tab)
}

// closeConnectOrigin closes the quick-connect page a successful connection
// was started from. Only that exact tab is removed: a page the user opened
// afterwards stays.
func (a *App) closeConnectOrigin(id int64) {
	if id == 0 {
		return
	}
	i := a.tabIndexByID(id)
	if i < 0 || a.tabs[i].kind != tabQuickConnect {
		return
	}
	a.tabs = append(a.tabs[:i], a.tabs[i+1:]...)
	if a.activeTab > i {
		a.activeTab--
	}
	a.syncTabs()
}

// retryConnectingTab dials the same server again after a failure, in the
// tab that already shows it.
func (a *App) retryConnectingTab(tab *appTab) {
	if a.connecting || tab == nil || tab.conn == nil {
		return
	}
	origin := tab.conn.origin
	a.connectErr = ""
	a.status = ""
	a.startConnectToTab(tab.conn.profile, tab, origin)
}

// startConnectTimer resets the progress shown for the in-flight connect and
// asks for a frame every 200ms so its elapsed time ticks.
func (a *App) startConnectTimer(tabID int64) {
	a.stopConnectTimer()
	a.connTabID = tabID
	a.connPhase = sshx.PhaseTCP
	a.connStarted = time.Now()

	stop := make(chan struct{})
	a.connTicker = stop
	go func() {
		tick := time.NewTicker(200 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-a.ctx.Done():
				return
			case <-tick.C:
				// An empty update is enough: the next frame recomputes the
				// elapsed time from connStarted.
				a.post(func() {})
			}
		}
	}()
}

func (a *App) stopConnectTimer() {
	if a.connTicker != nil {
		close(a.connTicker)
		a.connTicker = nil
	}
}

func connectPhaseLabel(ph sshx.Phase) string {
	switch ph {
	case sshx.PhaseTCP:
		return "正在建立 TCP 连接"
	case sshx.PhaseKeyExchange:
		return "正在协商加密算法"
	case sshx.PhaseHostKey:
		return "等待确认主机密钥"
	case sshx.PhaseAuth:
		return "正在验证账号与密码"
	case sshx.PhaseSession:
		return "正在打开终端会话"
	}
	return "正在连接"
}

// connectProgressText is the one-line progress the quick-connect page and
// the connecting tab share, e.g. "正在验证账号与密码 · 0.8s".
func (a *App) connectProgressText() string {
	text := connectPhaseLabel(a.connPhase)
	if !a.connStarted.IsZero() {
		text = fmt.Sprintf("%s · %.1fs", text, time.Since(a.connStarted).Seconds())
	}
	return text
}

// connectingTabView is what the tab shows while its saved connection dials:
// terminal-style lines at the top of the pane, not a dialog over it, so the
// tab already reads as the session that is about to appear.
func (a *App) connectingTabView(c *ui.Context, t *ui.Theme, tab *appTab) {
	conn := tab.conn
	if conn == nil {
		return
	}

	name := profileDisplayName(conn.profile)
	hostLine := fmt.Sprintf("%s@%s:%d", conn.profile.Username, conn.profile.Host, conn.profile.Port)

	ui.Column(c).Grow(1).Fill().Background(colTermBg).Padding(18, 20).Gap(5).Children(func() {
		ui.Text(c, "正在连接 "+shortLabel(name, 30)+" …").FontSize(11.5).
			Font("Cascadia Code").FontWeight(600).TextColor(colText).NoWrap()
		ui.Text(c, hostLine).FontSize(9.8).Font("Cascadia Code").
			TextColor(colMuted).NoWrap()

		if conn.err != "" {
			a.connectingTabError(c, t, tab, conn)
			return
		}

		ui.Row(c).Gap(7).AlignItems(ui.Center).Margin(9, 0, 0, 0).Children(func() {
			statusDot(c, colCyan, 6)
			ui.Text(c, a.connectProgressText()).FontSize(9.8).
				Font("Cascadia Code").TextColor(colCyan)
		})
		ui.Text(c, "连接成功后此标签页会变成终端；关闭标签页可取消").
			FontSize(9).Font("Cascadia Code").TextColor(colMuted2).Margin(2, 0, 0, 0)
	})
}

// connectingTabError replaces the progress with the failure and the two
// useful ways out: dial again, or fix the saved connection.
func (a *App) connectingTabError(c *ui.Context, t *ui.Theme, tab *appTab, conn *connectingTab) {
	ui.Text(c, "连接失败: "+shortLabel(conn.err, 150)).FontSize(10).
		Font("Cascadia Code").TextColor(colRed).LineHeight(1.6).Margin(9, 0, 0, 0)
	ui.Row(c).Gap(8).Margin(10, 0, 0, 0).Children(func() {
		if primaryAction(c, fmt.Sprintf("conn-retry-%d", tab.id), "重试", iconRefresh).Clicked() {
			a.retryConnectingTab(tab)
		}
		if ghostAction(c, t, fmt.Sprintf("conn-edit-%d", tab.id), "编辑连接", iconEdit).Clicked() {
			a.editProfile(conn.profile)
		}
	})
}
