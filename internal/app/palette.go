package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"xu/internal/store"
)

func (a *App) matchingProfiles() []store.Profile {
	q := strings.ToLower(strings.TrimSpace(a.searchQuery))
	if q == "" {
		return a.profiles
	}
	out := make([]store.Profile, 0, len(a.profiles))
	for _, p := range a.profiles {
		haystack := strings.ToLower(p.Name + " " + p.Host + " " + p.Username)
		if strings.Contains(haystack, q) {
			out = append(out, p)
		}
	}
	return out
}

func (a *App) searchModal(c *ui.Context, t *ui.Theme) {
	shade := ui.Hex("#000000").Alpha(0.58)
	if !a.uiDark() {
		shade = ui.Hex("#0f172a").Alpha(0.3)
	}
	matches := a.matchingProfiles()

	ui.DialogBase(c, &a.searchOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(shade)
		panel.Width(560).Padding(0).Background(colPanel).
			Border(1, colLine2).Radius(13).
			Shadow(0, 26, 70, 0, ui.Hex("#000000").Alpha(0.42)).Children(func() {
			ui.Row(c).Height(48).Shrink(0).Padding(0, 15).Gap(9).AlignItems(ui.Center).
				BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
				iconEl(c, iconSearch, 16, colMuted)
				ui.TextInput(c, &a.searchQuery).Grow(1).Height(22).FontSize(12).
					Placeholder("搜索会话、主机或用户…").AutoFocus().
					Background(ui.Transparent).Border(0, ui.Transparent)
				close := ui.ButtonBase(c.Key("search-close")).Size(24, 24).Radius(6).Center().
					Tooltip("关闭搜索").Label("关闭搜索")
				if close.Hovered() {
					close.Background(colPanel2)
				}
				close.Children(func() { iconEl(c, iconClose, 13, colMuted) })
				if close.Clicked() {
					a.searchOpen = false
				}
			})

			ui.Column(c).Padding(6, 6, 6, 6).Children(func() {
				if len(matches) == 0 {
					ui.Column(c).Padding(22, 14).Center().Gap(3).Children(func() {
						ui.Text(c, "没有匹配的连接").FontSize(10).TextColor(colMuted2)
						ui.Text(c, "换个名称、主机或用户名试试").FontSize(9).TextColor(colMuted2)
					})
					return
				}
				ui.Scroll(c).MaxHeight(420).Children(func() {
					for _, p := range matches {
						p := p
						a.searchResultRow(c, t, p)
					}
				})
			})

			ui.Row(c).Height(34).Shrink(0).Padding(0, 14).Gap(10).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(colLine).Children(func() {
				ui.Text(c, fmt.Sprintf("%d 个会话", len(a.profiles))).FontSize(8.4).
					Font("Cascadia Code").TextColor(colMuted2)
				ui.Text(c, "").Grow(1)
				if ghostAction(c, t, "search-new", "新建连接", iconPlus).Clicked() {
					a.searchOpen = false
					a.openConnectForm()
				}
				ui.Text(c, "Ctrl K").FontSize(8.4).Font("Cascadia Code").TextColor(colMuted2)
			})
		})
	})
}

func (a *App) searchResultRow(c *ui.Context, t *ui.Theme, p store.Profile) {
	b := ui.ButtonBase(c.Key(fmt.Sprintf("search-profile-%d", p.ID))).
		FillWidth().Height(36).Padding(0, 9).Radius(7).Gap(9).AlignItems(ui.Center)
	if b.Hovered() {
		b.Background(colAccentSoft)
	}
	b.Children(func() {
		ui.Box(c).Size(24, 24).Radius(6).Background(colPanel2).
			Border(1, colLine).Center().Children(func() {
			iconEl(c, iconServer, 12, colMuted)
		})
		ui.Column(c).Grow(1).Gap(0).Children(func() {
			ui.Text(c, p.Name).FontSize(10.5).FontWeight(550).TextColor(colText2).NoWrap()
			ui.Text(c, p.Username+"@"+p.Host+fmt.Sprintf(":%d", p.Port)).
				FontSize(8.2).Font("Cascadia Code").TextColor(colMuted2).NoWrap()
		})
		ui.Text(c, a.groupNameByID(p.GroupID)).FontSize(8.5).TextColor(colMuted2).NoWrap()
	})
	if b.Clicked() {
		a.searchOpen = false
		a.connectSavedProfile(p)
	}
}
