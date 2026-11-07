package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"xu/internal/store"
)

func (a *App) connectionsPage(c *ui.Context, t *ui.Theme) {
	visible := a.visibleProfiles()
	if len(visible) > 0 {
		found := false
		for _, p := range visible {
			if p.ID == a.connPageSel {
				found = true
				break
			}
		}
		if !found {
			a.connPageSel = visible[0].ID
		}
	}

	openCount := 0
	for _, ts := range a.terms {
		if !ts.local && !ts.exited {
			openCount++
		}
	}

	ui.Column(c).Grow(1).Fill().Padding(24, 28, 20, 28).Gap(14).Children(func() {
		pageHeader(c, t, iconConnections, "连接管理",
			fmt.Sprintf("%d 个连接 · %d 个已打开 · 凭据保存在系统钥匙串", len(a.profiles), openCount),
			func() {
				if ghostAction(c, t, "conn-page-refresh", "刷新", iconRefresh).Clicked() {
					a.refreshProfiles()
					a.status = "连接列表已刷新"
				}
				if primaryAction(c, "conn-page-new", "新建连接", iconPlus).Clicked() {
					a.openConnectForm()
				}
			})

		ui.Row(c).Grow(1).Gap(14).AlignItems(ui.Stretch).Children(func() {
			ui.Column(c).Grow(1).FillHeight().Background(colPanel).
				Border(1, colLine).Radius(8).Children(func() {
				ui.Row(c).Height(46).Shrink(0).Padding(0, 10).Gap(8).AlignItems(ui.Center).
					BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
					ui.TextInput(c, &a.connFilter).Grow(1).Height(30).FontSize(11).
						Placeholder("搜索名称、主机或用户名")
					a.connectionsFilterChips(c, t)
				})

				ui.Column(c).Grow(1).Padding(6, 4).Children(func() {
					ui.Scroll(c).Grow(1).Gap(1).Children(func() {
						if len(a.profiles) == 0 {
							a.connectionsEmpty(c, t)
							return
						}
						if len(visible) == 0 {
							ui.Column(c).Fill().Center().Gap(6).Padding(30).Children(func() {
								ui.Text(c, "没有匹配的连接").FontSize(fsBody).TextColor(colMuted)
								if ghostAction(c, t, "conn-page-clear", "清除筛选", nil).Clicked() {
									a.connFilter = ""
									a.authFilter = ""
								}
							})
							return
						}

						for _, group := range []string{"password", "key", "agent"} {
							var items []store.Profile
							for _, p := range visible {
								if authGroup(p.AuthType) == group {
									items = append(items, p)
								}
							}
							if len(items) == 0 {
								continue
							}
							a.shellSection(c, t, fmt.Sprintf("%s · %d", authLabel(group), len(items)))
							for _, p := range items {
								p := p
								a.connectionPageRow(c, t, p)
							}
						}
					})
				})
			})

			ui.Column(c).Width(300).Shrink(0).FillHeight().Background(colPanel).
				Border(1, colLine).Radius(8).Children(func() {
				a.connectionPageDetail(c, t)
			})
		})
	})
}

func (a *App) visibleProfiles() []store.Profile {
	q := strings.ToLower(strings.TrimSpace(a.connFilter))
	out := make([]store.Profile, 0, len(a.profiles))
	for _, p := range a.profiles {
		if a.authFilter != "" && authGroup(p.AuthType) != a.authFilter {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(p.Name+" "+p.Host+" "+p.Username), q) {
			continue
		}
		out = append(out, p)
	}
	return out
}

func authGroup(auth string) string {
	switch auth {
	case "key", "agent":
		return auth
	default:
		return "password"
	}
}

func authLabel(auth string) string {
	switch auth {
	case "password":
		return "密码登录"
	case "key":
		return "私钥登录"
	case "agent":
		return "Agent 登录"
	default:
		return "密码登录"
	}
}

func (a *App) connectionsFilterChips(c *ui.Context, t *ui.Theme) {
	counts := map[string]int{"": len(a.profiles)}
	for _, p := range a.profiles {
		counts[authGroup(p.AuthType)]++
	}
	items := []struct {
		key   string
		label string
	}{
		{"", "全部"},
		{"password", "密码"},
		{"key", "私钥"},
		{"agent", "Agent"},
	}
	for _, item := range items {
		item := item
		active := a.authFilter == item.key
		chip := ui.ButtonBase(c.Key("conn-chip-"+item.key)).Height(26).Padding(0, 8).
			Radius(4).Gap(5).AlignItems(ui.Center)
		if active {
			chip.Background(colCyan.Alpha(0.08)).Border(1, colCyan.Alpha(0.3))
		} else if chip.Hovered() {
			chip.Background(t.SurfaceHover)
		}
		chip.Children(func() {
			color := colMuted
			if active {
				color = colCyan
			}
			ui.Text(c, item.label).FontSize(10).TextColor(color).NoWrap()
			ui.Text(c, fmt.Sprintf("%d", counts[item.key])).FontSize(8.5).Font("Cascadia Code").
				TextColor(condColor(active, colCyan, colMuted2)).NoWrap()
		})
		if chip.Clicked() {
			a.authFilter = item.key
			a.connPageSel = 0
		}
	}
}

func (a *App) connectionsEmpty(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Fill().Center().Gap(9).Padding(30).Children(func() {
		ui.Box(c).Size(52, 52).Radius(12).
			Background(colPanel2).Border(1, colLine).Center().Children(func() {
			iconEl(c, iconConnections, 22, colMuted2)
		})
		ui.Text(c, "还没有保存的连接").FontSize(fsTitle).FontWeight(600).TextColor(colText)
		ui.Text(c, "新建一个 SSH 连接，之后可以在这里管理和打开").FontSize(10.5).TextColor(colMuted).
			TextAlign(ui.Center)
		if primaryAction(c, "conn-empty-new", "新建连接", iconPlus).Clicked() {
			a.openConnectForm()
		}
	})
}

func (a *App) connectionPageRow(c *ui.Context, t *ui.Theme, p store.Profile) {
	selected := p.ID == a.connPageSel
	connected := a.profileConnected(p.ID)
	ui.Row(c).FillWidth().Height(54).Padding(0, 5).Gap(2).AlignItems(ui.Center).Children(func() {
		main := ui.ButtonBase(c.Key(fmt.Sprintf("conn-row-%d", p.ID))).Grow(1).Height(50).
			Padding(0, 8).Radius(6).Gap(9).AlignItems(ui.Center)
		if selected {
			main.Background(colCyan.Alpha(0.06))
			main.BorderWidth(0, 0, 0, 2).BorderColor(colCyan)
		} else if main.Hovered() {
			main.Background(t.SurfaceHover)
		}
		main.Children(func() {
			statusDot(c, condColor(connected, colGreen, colMuted2), 6)
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				ui.Text(c, shortLabel(p.Name, 24)).FontSize(11.5).FontWeight(600).
					TextColor(colText).NoWrap()
				ui.Text(c, fmt.Sprintf("%s@%s:%d", p.Username, p.Host, p.Port)).FontSize(9.5).
					Font("Cascadia Code").TextColor(colMuted).NoWrap()
			})
			if connected {
				miniTag(c, "LIVE", colGreen)
			}
			miniTag(c, authLabel(p.AuthType), colMuted2)
			iconEl(c, iconChevronRight, 12, colMuted2)
		})
		if main.Clicked() {
			a.connPageSel = p.ID
		}
		if iconButton(c, t, fmt.Sprintf("conn-row-edit-%d", p.ID), iconEdit, "编辑连接").Clicked() {
			a.editProfile(p)
		}
	})
}

func (a *App) selectedProfile() *store.Profile {
	if a.connPageSel == 0 {
		return nil
	}
	for i := range a.profiles {
		if a.profiles[i].ID == a.connPageSel {
			return &a.profiles[i]
		}
	}
	return nil
}

func (a *App) connectionPageDetail(c *ui.Context, t *ui.Theme) {
	p := a.selectedProfile()
	if p == nil {
		ui.Column(c).Fill().Center().Gap(7).Padding(24).Children(func() {
			iconEl(c, iconServer, 24, colMuted2)
			ui.Text(c, "选择一个连接").FontSize(fsBody).TextColor(colMuted)
			ui.Text(c, "查看地址、认证方式并打开终端").FontSize(10).TextColor(colMuted2)
		})
		return
	}
	connected := a.profileConnected(p.ID)
	statusLabel := "就绪"
	if connected {
		statusLabel = "已打开"
	}

	ui.Column(c).FillHeight().Children(func() {
		ui.Column(c).Padding(16, 16, 14, 16).Background(colCyan.Alpha(0.06)).
			BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				statusDot(c, condColor(connected, colGreen, colMuted2), 6)
				ui.Text(c, "SSH").FontSize(9).Font("Cascadia Code").
					TextColor(colMuted2)
				ui.Text(c, "").Grow(1)
				ui.Text(c, fmt.Sprintf(":%d", p.Port)).FontSize(9).Font("Cascadia Code").
					TextColor(colMuted2)
			})
			ui.Text(c, shortLabel(p.Name, 28)).FontSize(18).FontWeight(650).
				TextColor(colText).Padding(0, 0, 4, 0)
			ui.Text(c, fmt.Sprintf("%s@%s", p.Username, p.Host)).FontSize(10).
				Font("Cascadia Code").TextColor(colMuted).NoWrap()
			ui.Row(c).Gap(5).Padding(0, 10, 0, 0).Children(func() {
				a.invertTag(c, authLabel(p.AuthType))
				if connected {
					a.invertTag(c, "已打开")
				}
			})
		})

		ui.Row(c).Padding(10, 12).Gap(6).Children(func() {
			open := primaryAction(c, "conn-detail-open", "打开终端", iconTerm).Grow(1)
			if open.Clicked() {
				a.connectSavedProfile(*p)
			}
			if ghostAction(c, t, "conn-detail-edit", "编辑", iconEdit).Clicked() {
				a.editProfile(*p)
			}
		})

		ui.Scroll(c).Grow(1).Padding(10, 14).Children(func() {
			ui.Column(c).Gap(0).Children(func() {
				a.connFact(c, t, "主机", p.Host, true)
				a.connFact(c, t, "端口", fmt.Sprintf("%d", p.Port), true)
				a.connFact(c, t, "用户名", p.Username, true)
				a.connFact(c, t, "认证方式", authLabel(p.AuthType), false)
				a.connFact(c, t, "状态", statusLabel, false)
			})
			ui.Column(c).Padding(0, 10).Gap(6).Children(func() {
				ui.Text(c, "主机密钥").FontSize(9).FontWeight(600).TextColor(colMuted2)
				ui.Text(c, "首次连接时显示 SHA256 指纹，确认后写入 known_hosts。密钥变化会阻断连接。").
					FontSize(9.5).TextColor(colMuted2).LineHeight(1.55)
			})
			ui.Column(c).Padding(0, 10).Gap(6).Children(func() {
				ui.Text(c, "凭据").FontSize(9).FontWeight(600).TextColor(colMuted2)
				ui.Text(c, "密码保存在系统钥匙串；应用数据库不保存明文。").
					FontSize(9.5).TextColor(colMuted2).LineHeight(1.55)
			})
		})
	})
}

func (a *App) invertTag(c *ui.Context, label string) {
	miniTag(c, label, colCyan)
}

func (a *App) connFact(c *ui.Context, t *ui.Theme, label, value string, mono bool) {
	ui.Row(c).Height(31).BorderWidth(0, 0, 1, 0).BorderColor(colLine).
		AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(10).TextColor(colMuted2)
		ui.Text(c, "").Grow(1)
		txt := ui.Text(c, shortLabel(value, 30)).FontSize(10.5).FontWeight(500).
			TextColor(colText).NoWrap()
		if mono {
			txt.Font("Cascadia Code").FontSize(10)
		}
	})
}
