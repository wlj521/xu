package app

import (
	"fmt"
	"net"
	"strconv"

	"github.com/egoist/mygo/ui"

	"xu/internal/store"
)

func knownHostAddress(host string, port int) string {
	return net.JoinHostPort(host, strconv.Itoa(port))
}

// loadKnownHosts initialises both the UI list and the live trust map. It runs
// before any connections are opened.
func (a *App) loadKnownHosts() {
	if a.store == nil {
		a.knownHosts = nil
		a.knownHostsErr = "会话库未就绪，暂时无法管理主机密钥"
		return
	}
	records, err := a.store.KnownHostRecords(a.ctx)
	if err != nil {
		a.knownHostsErr = "读取主机密钥失败: " + err.Error()
		a.status = a.knownHostsErr
		return
	}

	trusted := make(map[string]string, len(records))
	for _, record := range records {
		trusted[knownHostAddress(record.Host, record.Port)] = record.Fingerprint
	}
	a.trustMu.Lock()
	a.trusted = trusted
	a.trustMu.Unlock()

	a.knownHosts = records
	a.knownHostsErr = ""
}

// refreshKnownHosts updates the management list without replacing the live
// trust map, which background handshakes may already be using.
func (a *App) refreshKnownHosts() {
	if a.store == nil {
		a.knownHosts = nil
		a.knownHostsErr = "会话库未就绪，暂时无法管理主机密钥"
		return
	}
	records, err := a.store.KnownHostRecords(a.ctx)
	if err != nil {
		a.knownHostsErr = "读取主机密钥失败: " + err.Error()
		return
	}
	a.knownHosts = records
	a.knownHostsErr = ""
}

func (a *App) openKnownHosts() {
	a.refreshKnownHosts()
	a.knownHostsOpen = true
}

func (a *App) closeKnownHosts() {
	a.knownHostsOpen = false
}

func (a *App) deleteKnownHost(record store.KnownHost) {
	if a.store == nil {
		a.knownHostsErr = "会话库未就绪，暂时无法删除主机密钥"
		return
	}
	if err := a.store.DeleteKnownHost(a.ctx, record.Host, record.Port); err != nil {
		a.knownHostsErr = "删除主机密钥失败: " + err.Error()
		return
	}

	address := knownHostAddress(record.Host, record.Port)
	a.trustMu.Lock()
	delete(a.trusted, address)
	a.trustMu.Unlock()
	a.refreshKnownHosts()
	a.status = "已删除主机密钥 " + address
}

func (a *App) knownHostsModal(c *ui.Context, t *ui.Theme) {
	if !a.knownHostsOpen {
		return
	}

	shade := ui.Hex("#000000").Alpha(0.58)
	if !a.uiDark() {
		shade = ui.Hex("#0f172a").Alpha(0.3)
	}
	w, h := fitDialog(c, 680, 540)

	ui.DialogBase(c, &a.knownHostsOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(shade)
		panel.Width(w).Height(h).Padding(0).Background(colPanel).
			Border(1, colLine2).Radius(14).
			Shadow(0, 26, 70, 0, ui.Hex("#000000").Alpha(0.42)).Children(func() {
			ui.Column(c).FillHeight().Children(func() {
				a.knownHostsHeader(c, t)
				ui.Scroll(c).Grow(1).Children(func() {
					ui.Column(c).FillWidth().Padding(16, 18).Gap(10).Children(func() {
						a.knownHostsBody(c)
					})
				})
				a.knownHostsFooter(c, t)
			})
		})
	})
}

func (a *App) knownHostsHeader(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Height(64).Shrink(0).Padding(0, 18).Gap(11).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
		ui.Box(c).Size(30, 30).Radius(8).
			Background(colCyan.Alpha(0.1)).Border(1, colCyan.Alpha(0.26)).Center().Children(func() {
			iconEl(c, iconServer, 15, colCyan)
		})
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, "已知主机").FontSize(14.5).FontWeight(650)
			ui.Text(c, fmt.Sprintf("已保存 %d 个 SSH 主机密钥 · known_hosts", len(a.knownHosts))).
				FontSize(9.5).TextColor(colMuted).NoWrap()
		})
		if iconButton(c, t, "known-hosts-close", iconClose, "关闭").Clicked() {
			a.closeKnownHosts()
		}
	})
}

func (a *App) knownHostsBody(c *ui.Context) {
	if a.knownHostsErr != "" {
		ui.Box(c).FillWidth().Padding(9, 10).Radius(7).
			Background(colRed.Alpha(0.08)).Border(1, colRed.Alpha(0.2)).Children(func() {
			ui.Text(c, a.knownHostsErr).FontSize(10).TextColor(colRed)
		})
	}

	ui.Row(c).FillWidth().Padding(9, 10).Gap(7).Radius(7).
		Background(colAmber.Alpha(0.07)).Border(1, colAmber.Alpha(0.2)).
		AlignItems(ui.Start).Children(func() {
		iconEl(c, hostKeyWarnIcon, 12, colAmber)
		ui.Text(c, "连接时若主机指纹变化，Xu 会同时显示旧值和新值；只有再次确认后才会更新记录。").
			FontSize(10).TextColor(colAmber).LineHeight(1.45)
	})

	if len(a.knownHosts) == 0 {
		settingsNote(c, iconShield, "还没有保存的主机密钥。首次连接服务器并核对 SHA256 指纹后，记录会出现在这里。")
		return
	}

	for _, record := range a.knownHosts {
		record := record
		address := knownHostAddress(record.Host, record.Port)
		ui.Column(c).FillWidth().Padding(11, 12).Gap(7).Radius(8).
			Background(colPanel2.Alpha(0.55)).Border(1, colLine).Children(func() {
			ui.Row(c).FillWidth().Gap(9).AlignItems(ui.Center).Children(func() {
				iconEl(c, iconServer, 13, colCyan)
				ui.Text(c, address).FontSize(11.5).FontWeight(600).
					Font("Cascadia Code").TextColor(colText2)
				ui.Text(c, "").Grow(1)
				if dangerAction(c, fmt.Sprintf("known-host-delete-%s-%d", record.Host, record.Port),
					"删除", iconTrash).Clicked() {
					a.deleteKnownHost(record)
				}
			})
			ui.Text(c, record.Fingerprint).FontSize(9.8).Font("Cascadia Code").
				LineHeight(1.5).TextColor(colMuted).Selectable()
		})
	}
}

func (a *App) knownHostsFooter(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Height(56).Shrink(0).Padding(0, 18).Gap(8).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(colLine).Children(func() {
		iconEl(c, iconShield, 11, colGreen)
		ui.Text(c, "删除后，下次连接会重新核对指纹。").FontSize(9).TextColor(colMuted2).NoWrap()
		ui.Text(c, "").Grow(1)
		if ghostAction(c, t, "known-hosts-done", "关闭", nil).Clicked() {
			a.closeKnownHosts()
		}
	})
}
