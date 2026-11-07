package app

import (
	"github.com/egoist/mygo/ui"
)

const titleBarHeight = 48

func (a *App) titleBar(c *ui.Context, t *ui.Theme, bar ui.TitleBar) {
	h := bar.Height
	if h < titleBarHeight {
		h = titleBarHeight
	}

	ui.Row(c).Height(h).Shrink(0).AlignItems(ui.Center).
		Background(colPanelTop).
		BorderWidth(0, 0, 1, 0).BorderColor(colLine).
		Padding(0, bar.Right+10, 0, 0).Children(func() {
		a.titleBrand(c, t, h)
		a.titleChip(c, t, h)
		a.titleActions(c, t, bar)
	})
}

func (a *App) titleBrand(c *ui.Context, t *ui.Theme, h float32) {
	ui.Row(c).Width(264).Height(h).Padding(0, 14).Gap(10).AlignItems(ui.Center).
		BorderWidth(0, 1, 0, 0).BorderColor(colLine).DragWindow().Children(func() {
		ui.Box(c).Width(27).Height(27).Radius(8).
			Gradient(ui.Hex("#B2433A"), ui.Hex("#6E1A1D"), 135).
			Border(1, ui.Hex("#F5E8D2").Alpha(0.24)).Center().Children(func() {
			iconEl(c, iconSealXu, 18, ui.Hex("#F5E8D2"))
		})
		ui.Column(c).Gap(1).Children(func() {
			ui.Text(c, "Xu").FontSize(13).FontWeight(650).LetterSpacing(0.2).TextColor(colText).NoWrap()
			ui.Text(c, "SSH 工作台").FontSize(7.5).FontWeight(650).
				LetterSpacing(1.4).TextColor(colMuted2).NoWrap()
		})
		ui.Text(c, "").Grow(1)
		toggleTip := "收起左侧连接栏（Ctrl Alt B）"
		toggleLabel := "收起左侧连接栏"
		if a.sidebarCollapsed {
			toggleTip = "展开左侧连接栏（Ctrl Alt B）"
			toggleLabel = "展开左侧连接栏"
		}
		collapse := ui.ButtonBase(c.Key("tb-collapse-sidebar")).Size(24, 24).Radius(6).Center().
			Tooltip(toggleTip).Label(toggleLabel)
		collapse.Background(colPanel2)
		collapse.Border(1, colLine)
		if collapse.Hovered() {
			collapse.Background(colAccentSoft)
			collapse.BorderColor(colAccentLine)
		}
		collapse.Children(func() {
			chevron := iconChevronLeft
			if a.sidebarCollapsed {
				chevron = iconChevronRight
			}
			iconEl(c, chevron, 12, colMuted)
		})
		if collapse.Clicked() {
			a.sidebarCollapsed = !a.sidebarCollapsed
			if a.sidebarCollapsed {
				a.status = "左侧连接栏已收起"
			} else {
				a.status = "左侧连接栏已展开"
			}
		}
	})
}

func (a *App) titleChip(c *ui.Context, t *ui.Theme, h float32) {
	ui.Box(c).Grow(1).Height(h).DragWindow().Center().Children(func() {
		b := ui.ButtonBase(c.Key("title-search")).Width(420).Height(30).
			Padding(0, 10).Radius(8).Gap(8).AlignItems(ui.Center)
		b.Background(colPanel).Border(1, colLine)
		if b.Hovered() {
			b.Background(colPanel2)
			b.Border(1, colLine2)
		}
		b.Children(func() {
			iconEl(c, iconSearch, 14, colMuted)
			ui.Text(c, "搜索会话、主机或用户").FontSize(10.5).TextColor(colMuted).
				Grow(1).NoWrap()
			ui.Box(c).Height(17).Padding(0, 5).Radius(4).Background(colPanelTop).
				Border(1, colLine).Center().Children(func() {
				ui.Text(c, "Ctrl K").FontSize(7.5).Font("Cascadia Code").TextColor(colMuted2)
			})
		})
		if b.Clicked() {
			a.searchOpen = true
			a.searchQuery = ""
		}
	})
}

func (a *App) titleActions(c *ui.Context, t *ui.Theme, bar ui.TitleBar) {
	ui.Row(c).Height(bar.Height).Gap(4).AlignItems(ui.Center).Padding(0, 10).Children(func() {
		ui.Row(c).Height(30).Padding(0, 2).Gap(2).Radius(8).AlignItems(ui.Center).
			Background(colPanel).Border(1, colLine).Children(func() {
			sun := ui.ButtonBase(c.Key("tb-light")).Width(27).Height(24).Radius(6).Center().Tooltip("浅色主题")
			if !a.uiDark() {
				sun.Background(colSurfaceHover)
				sun.Border(1, colLine)
			} else if sun.Hovered() {
				sun.Background(t.SurfaceHover)
			}
			sun.Children(func() {
				color := colMuted
				if !a.uiDark() {
					color = colText
				}
				iconEl(c, iconSun, 13, color)
			})
			if sun.Clicked() {
				a.setThemeMode(2)
			}

			moon := ui.ButtonBase(c.Key("tb-dark")).Width(27).Height(24).Radius(6).Center().Tooltip("深色主题")
			if a.uiDark() {
				moon.Background(colSurfaceHover)
				moon.Border(1, colLine)
			} else if moon.Hovered() {
				moon.Background(t.SurfaceHover)
			}
			moon.Children(func() {
				color := colMuted
				if a.uiDark() {
					color = colText
				}
				iconEl(c, iconMoon, 13, color)
			})
			if moon.Clicked() {
				a.setThemeMode(1)
			}
		})

		ui.Box(c).Width(1).Height(18).Background(colLine).MarginX(3)

		if iconButton(c, t, "tb-settings", iconGear, "设置（Ctrl Alt S）").Clicked() {
			a.openSettings()
		}
	})
}
