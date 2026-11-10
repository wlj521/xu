package app

import (
	"github.com/egoist/mygo/ui"
)

const titleBarHeight = 32

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
		a.titleActions(c, t, h)
	})
}

func (a *App) titleBrand(c *ui.Context, t *ui.Theme, h float32) {
	ui.Row(c).Width(monitorWidth(c)).Height(h).Padding(0, 14).Gap(10).AlignItems(ui.Center).
		BorderWidth(0, 1, 0, 0).BorderColor(colLine).DragWindow().Children(func() {
		ui.Image(c, titleIcon).Size(27, 27).Fit(ui.Contain).Shrink(0)
		ui.Column(c).Gap(1).Children(func() {
			ui.Text(c, "Xu").FontSize(13).FontWeight(650).LetterSpacing(0.2).TextColor(colText).NoWrap()
			ui.Text(c, "SSH 工作台").FontSize(7.5).FontWeight(650).
				LetterSpacing(1.4).TextColor(colMuted2).NoWrap()
		})
	})
}

func (a *App) titleChip(c *ui.Context, t *ui.Theme, h float32) {
	ui.Row(c).Grow(1).Height(h).AlignItems(ui.Center).Padding(0, 16).DragWindow().Children(func() {
		b := ui.ButtonBase(c.Key("title-search")).Grow(1).MaxWidth(420).Height(28).
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

func (a *App) titleActions(c *ui.Context, t *ui.Theme, h float32) {
	ui.Row(c).Height(h).Gap(4).AlignItems(ui.Center).Padding(0, 10).Children(func() {
		ui.Row(c).Height(28).Padding(0, 2).Gap(2).Radius(8).AlignItems(ui.Center).
			Background(colPanel).Border(1, colLine).Children(func() {
			sun := ui.ButtonBase(c.Key("tb-light")).Width(27).Height(22).Radius(6).Center().Tooltip("浅色主题")
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

			moon := ui.ButtonBase(c.Key("tb-dark")).Width(27).Height(22).Radius(6).Center().Tooltip("深色主题")
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
