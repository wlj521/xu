package app

import (
	"fmt"
	"net"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"xu/internal/store"
	"xu/internal/tunnelx"
)

type forwardEntry struct {
	forward *tunnelx.Forward
	kind    int
	listen  string
	target  string
}

func (a *App) openForwards() {
	if a.editorOpen {
		a.status = "请先保存或关闭编辑器"
		return
	}
	ts := a.activeTerminal()
	if ts == nil {
		a.status = "请先打开一个终端"
		return
	}
	if ts.local {
		a.status = "本地终端不支持端口转发"
		return
	}

	if a.finding {
		a.closeFind()
	}
	a.setDockTab(dockTunnels)
	a.forwardErr = ""
	a.reloadDockForwards()
}

func (a *App) forwardView(c *ui.Context, t *ui.Theme) {
	ts := a.activeTerminal()
	if ts == nil {
		a.forwardOpen = false
		return
	}
	list := a.forwards[ts.ID]

	ui.Column(c).Grow(1).Padding(24, 28, 20, 28).Gap(14).Children(func() {
		pageHeader(c, t, iconLink, "端口转发", ts.Title, func() {
			if ghostAction(c, t, "forward-terminal", "终端", iconTerm).Clicked() {
				a.forwardOpen = false
			}
			if ghostAction(c, t, "forward-files", "文件", iconFolder).Clicked() {
				a.forwardOpen = false
				a.filesOpen = true
				a.openFiles()
			}
			if ghostAction(c, t, "forward-monitor", "监控", iconActivity).Clicked() {
				a.forwardOpen = false
				a.openMonitor()
			}
		})

		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(14).Children(func() {
				cardPanel(c, func() {
					sectionLabel(c, "新建隧道")
					ui.Tabs(c, &a.forwardKind, "本地转发", "远程转发", "SOCKS5")

					description := ""
					switch a.forwardKind {
					case 0:
						description = "本机监听 → 远端目标，例如把远端 MySQL 映射到本机 3306。"
					case 1:
						description = "远端监听 → 本机目标，例如把本机 80 暴露到服务器的 9000。"
					case 2:
						description = "本机 SOCKS5 代理，浏览器代理指向监听地址即可；目标地址不用填。"
					}
					ui.Text(c, description).FontSize(10.5).TextColor(colMuted).LineHeight(1.5)

					ui.Column(c).Width(560).Gap(10).Children(func() {
						fieldColumn(c, "监听地址", func() {
							ui.TextInput(c, &a.forwardListen).Label("监听地址").Placeholder("127.0.0.1:8080")
						})
						if a.forwardKind != 2 {
							fieldColumn(c, "目标地址", func() {
								ui.TextInput(c, &a.forwardTarget).Label("目标地址").
									Placeholder("10.0.0.5:3306 或 127.0.0.1:80")
							})
						}

						ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
							ui.Checkbox(c, &a.forwardSave, "保存此规则")
							ui.Checkbox(c, &a.forwardAuto, "下次连接自动启动")
						})
						if ts.profileID == 0 {
							ui.Text(c, "当前会话未保存，转发规则只对本次运行有效。").
								FontSize(10.5).TextColor(colMuted2)
						}

						if a.forwardErr != "" {
							ui.Box(c).FillWidth().Padding(9, 11).Radius(8).
								Background(colRed.Alpha(0.07)).Border(1, colRed.Alpha(0.18)).Children(func() {
								ui.Text(c, a.forwardErr).FontSize(10.5).TextColor(colRed)
							})
						}
						if strings.HasPrefix(a.forwardListen, "0.0.0.0:") || strings.HasPrefix(a.forwardListen, ":") {
							ui.Box(c).FillWidth().Padding(9, 11).Radius(8).
								Background(colAmber.Alpha(0.07)).Border(1, colAmber.Alpha(0.18)).Children(func() {
								ui.Text(c, "警告：正在监听所有网卡，请确认网络环境可信。").
									FontSize(10.5).TextColor(colAmber)
							})
						}

						if primaryAction(c, "forward-start", "启动隧道", iconLink).Clicked() {
							a.startForward(ts)
						}
					})
				})

				cardPanel(c, func() {
					ui.Row(c).AlignItems(ui.Center).Children(func() {
						ui.Text(c, "运行中的隧道").FontSize(fsBody).FontWeight(600)
						ui.Text(c, "").Grow(1)
						miniTag(c, fmt.Sprintf("%d 条", len(list)), colGreen)
					})
					if len(list) == 0 {
						ui.Text(c, "还没有运行中的隧道").FontSize(10.5).TextColor(colMuted2)
					} else {
						for i, e := range list {
							i, e := i, e
							ui.Row(c).Height(42).Gap(10).AlignItems(ui.Center).Children(func() {
								miniTag(c, forwardKindName(e.kind), colCyan)
								ui.Text(c, forwardRowText(e)).Grow(1).FontSize(11).Font("Cascadia Code").NoWrap()
								rx, tx := e.forward.Bytes()
								ui.Text(c, fmt.Sprintf("↓ %s  ↑ %s", humanSize(rx), humanSize(tx))).
									FontSize(9.5).Font("Cascadia Code").TextColor(colMuted2).NoWrap()
								if ghostAction(c, t, fmt.Sprintf("forward-stop-%d", i), "停止", nil).Clicked() {
									a.stopForward(ts.ID, i)
								}
							})
						}
					}
				})

				cardPanel(c, func() {
					ui.Row(c).AlignItems(ui.Center).Children(func() {
						ui.Text(c, "已保存的规则").FontSize(fsBody).FontWeight(600)
						ui.Text(c, "").Grow(1)
						miniTag(c, fmt.Sprintf("%d 条", len(a.savedForwards)), colMuted)
					})
					if len(a.savedForwards) == 0 {
						ui.Text(c, "还没有保存的规则").FontSize(10.5).TextColor(colMuted2)
						return
					}
					for i, r := range a.savedForwards {
						i, r := i, r
						ui.Row(c).Height(42).Gap(10).AlignItems(ui.Center).Children(func() {
							miniTag(c, forwardKindName(r.Kind), colBlue)
							ui.Text(c, savedForwardText(r)).Grow(1).FontSize(11).Font("Cascadia Code").NoWrap()
							if r.AutoStart {
								miniTag(c, "自动", colGreen)
							}
							if quietAction(c, t, fmt.Sprintf("forward-start-saved-%d", i), "启动", nil, false).Clicked() {
								a.startForwardSpec(ts, r.Kind, r.Listen, r.Target, false)
							}
							if quietAction(c, t, fmt.Sprintf("forward-delete-saved-%d", i), "删除", nil, false).Clicked() {
								a.deleteSavedForward(r.ID, ts)
							}
						})
					}
				})
			})
		})
	})
}

func (a *App) startForward(ts *termSession) {
	a.startForwardSpec(
		ts,
		a.forwardKind,
		strings.TrimSpace(a.forwardListen),
		strings.TrimSpace(a.forwardTarget),
		a.forwardSave,
	)
}

func (a *App) startForwardSpec(ts *termSession, kind int, listen, target string, save bool) {
	if listen == "" {
		a.forwardErr = "请填写监听地址"
		return
	}
	if kind != 2 {
		if target == "" {
			a.forwardErr = "请填写目标地址"
			return
		}
		if _, _, err := net.SplitHostPort(target); err != nil {
			a.forwardErr = "目标地址格式应为 host:port"
			return
		}
	}
	if a.forwardExists(ts.ID, kind, listen, target) {
		a.forwardErr = "相同转发已在运行"
		return
	}

	var (
		f   *tunnelx.Forward
		err error
	)
	switch kind {
	case 0:
		f, err = tunnelx.StartLocal(ts.client, listen, target)
	case 1:
		f, err = tunnelx.StartRemote(ts.client, listen, target)
	case 2:
		f, err = tunnelx.StartSOCKS5(ts.client, listen)
	default:
		a.forwardErr = "未知转发类型"
		return
	}
	if err != nil {
		a.forwardErr = "启动失败: " + err.Error()
		return
	}

	a.forwards[ts.ID] = append(a.forwards[ts.ID], &forwardEntry{
		forward: f,
		kind:    kind,
		listen:  listen,
		target:  target,
	})
	a.forwardErr = ""

	if !save {
		return
	}
	if a.store == nil || ts.profileID == 0 {
		a.status = "当前会话未保存，转发规则不会持久化"
		return
	}

	rule := store.ForwardRule{
		ProfileID: ts.profileID,
		Kind:      kind,
		Listen:    listen,
		Target:    target,
		AutoStart: a.forwardAuto,
	}
	if err := a.store.SaveForward(a.ctx, &rule); err != nil {
		a.forwardErr = "启动成功，但保存规则失败: " + err.Error()
		return
	}
	a.reloadSavedForwards(ts)
}

func (a *App) forwardExists(termID int64, kind int, listen, target string) bool {
	for _, e := range a.forwards[termID] {
		if e.kind == kind && e.listen == listen && e.target == target {
			return true
		}
	}
	return false
}

func (a *App) reloadSavedForwards(ts *termSession) {
	if a.store == nil || ts == nil || ts.profileID == 0 {
		a.savedForwards = nil
		return
	}
	rules, err := a.store.Forwards(a.ctx, ts.profileID)
	if err != nil {
		a.forwardErr = "读取转发规则失败: " + err.Error()
		return
	}
	a.savedForwards = rules
}

func (a *App) deleteSavedForward(id int64, ts *termSession) {
	if a.store == nil {
		return
	}

	res, _ := mygo.Dialog.Message(mygo.MessageOptions{
		Type:               mygo.MessageWarning,
		Message:            "删除这条转发规则？",
		Buttons:            []string{"取消", "删除"},
		DestructiveButtons: []int{1},
	})
	if res.Button != 1 {
		return
	}

	if err := a.store.DeleteForward(a.ctx, id); err != nil {
		a.forwardErr = "删除失败: " + err.Error()
		return
	}
	a.reloadSavedForwards(ts)
}

func (a *App) autoStartForwards(ts *termSession) {
	if a.store == nil || ts == nil || ts.profileID == 0 {
		return
	}
	rules, err := a.store.Forwards(a.ctx, ts.profileID)
	if err != nil {
		return
	}
	for _, r := range rules {
		if !r.AutoStart {
			continue
		}
		// A second tab of the same server must not fight the first one for
		// the same local (or remote) port.
		if a.forwardListenInUse(r.Listen) {
			continue
		}
		a.startForwardSpec(ts, r.Kind, r.Listen, r.Target, false)
		if a.forwardErr != "" {
			a.status = "自动启动转发失败: " + a.forwardErr
		}
	}
}

// forwardListenInUse reports whether any live session already listens on
// addr, across all sessions and profiles.
func (a *App) forwardListenInUse(listen string) bool {
	for _, list := range a.forwards {
		for _, e := range list {
			if e.listen == listen {
				return true
			}
		}
	}
	return false
}

func (a *App) stopForward(termID int64, i int) {
	list := a.forwards[termID]
	if i < 0 || i >= len(list) {
		return
	}
	list[i].forward.Close()
	list = append(list[:i], list[i+1:]...)
	if len(list) == 0 {
		delete(a.forwards, termID)
		return
	}
	a.forwards[termID] = list
}

func (a *App) stopForwards(termID int64) {
	for _, e := range a.forwards[termID] {
		e.forward.Close()
	}
	delete(a.forwards, termID)
}

func (a *App) closeAllForwards() {
	for termID := range a.forwards {
		a.stopForwards(termID)
	}
}

func forwardKindName(kind int) string {
	switch kind {
	case 0:
		return "本地"
	case 1:
		return "远程"
	case 2:
		return "SOCKS5"
	}
	return "未知"
}

func forwardRowText(e *forwardEntry) string {
	if e.kind == 2 {
		return e.forward.Addr() + " → SOCKS5"
	}
	return e.forward.Addr() + " → " + e.target
}

func savedForwardText(r store.ForwardRule) string {
	if r.Kind == 2 {
		return r.Listen + " → SOCKS5"
	}
	return r.Listen + " → " + r.Target
}
