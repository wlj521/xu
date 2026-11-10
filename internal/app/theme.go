package app

import (
	"strconv"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// Light mode keeps the porcelain "Switchboard" surfaces. Dark mode follows
// JetBrains' new UI hierarchy: charcoal panels, quiet borders and a softer,
// familiar terminal palette. Status colours stay reserved for their meaning.
var (
	colBg             ui.Color
	colPanelTop       ui.Color
	colPanel          ui.Color
	colPanelDeep      ui.Color
	colPanel2         ui.Color
	colCard           ui.Color
	colCardTop        ui.Color
	colLine           ui.Color
	colLine2          ui.Color
	colText           ui.Color
	colText2          ui.Color
	colMuted          ui.Color
	colMuted2         ui.Color
	colCyan           ui.Color
	colViolet         ui.Color
	colGreen          ui.Color
	colAmber          ui.Color
	colRed            ui.Color
	colBlue           ui.Color
	colTrack          ui.Color
	colInk            ui.Color
	colTermTop        ui.Color
	colTermBg         ui.Color
	colTermLine       ui.Color
	colTreeBg         ui.Color
	colFileLink       ui.Color
	colFilePerm       ui.Color
	colAccentText     ui.Color
	colAccentSoft     ui.Color
	colAccentLine     ui.Color
	colSurfaceHover   ui.Color
	colSurfacePressed ui.Color
	colAccentHover    ui.Color
	colAccentPressed  ui.Color
	colScrollbar      ui.Color
)

func setPalette(dark bool) {
	if dark {
		colBg = ui.Hex("#18191C")
		colPanelTop = ui.Hex("#26282C")
		colPanel = ui.Hex("#2B2D30")
		colPanelDeep = ui.Hex("#1E1F22")
		colPanel2 = ui.Hex("#313338")
		colCard = ui.Hex("#2B2D30")
		colCardTop = ui.Hex("#2B2D30")
		colLine = ui.Hex("#34373C")
		colLine2 = ui.Hex("#4A4D53")
		colText = ui.Hex("#DFE1E5")
		colText2 = ui.Hex("#BCBEC4")
		colMuted = ui.Hex("#8C8F94")
		colMuted2 = ui.Hex("#6F737A")
		colCyan = ui.Hex("#4D8DFF")
		colViolet = ui.Hex("#B49BD8")
		colGreen = ui.Hex("#6AAB73")
		colAmber = ui.Hex("#CFA85A")
		colRed = ui.Hex("#E06C75")
		colBlue = ui.Hex("#4C9CFF")
		colTrack = ui.RGBA(255, 255, 255, 0.08)
		colInk = ui.Hex("#1E1F22")
		colTermTop = ui.Hex("#26282C")
		colTermBg = ui.Hex("#1E1F22")
		colTermLine = ui.Hex("#34373C")
		colTreeBg = ui.Hex("#1E1F22")
		colFileLink = ui.Hex("#9CB7E5")
		colFilePerm = ui.Hex("#7EA7D8")
		colAccentText = ui.Hex("#FFFFFF")
		colSurfaceHover = ui.Hex("#2E3035")
		colSurfacePressed = ui.Hex("#35383D")
		colAccentHover = ui.Hex("#6BA3FF")
		colAccentPressed = ui.Hex("#3C7BF0")
		colScrollbar = ui.RGBA(255, 255, 255, 0.18)
		return
	}

	colBg = ui.Hex("#E9EDF4")
	colPanelTop = ui.Hex("#F4F7FB")
	colPanel = ui.Hex("#FFFFFF")
	colPanelDeep = ui.Hex("#FFFFFF")
	colPanel2 = ui.Hex("#FFFFFF")
	colCard = ui.Hex("#FFFFFF")
	colCardTop = ui.Hex("#FFFFFF")
	colLine = ui.Hex("#E4E9F1")
	colLine2 = ui.Hex("#D4DCE8")
	colText = ui.Hex("#151D2E")
	colText2 = ui.Hex("#44506A")
	colMuted = ui.Hex("#7C889C")
	colMuted2 = ui.Hex("#A3ADBE")
	colCyan = ui.Hex("#2F6FEB")
	colViolet = ui.Hex("#7C5CF0")
	colGreen = ui.Hex("#0F9D6B")
	colAmber = ui.Hex("#B7791F")
	colRed = ui.Hex("#D94357")
	colBlue = ui.Hex("#0891B2")
	colTrack = ui.RGBA(21, 29, 46, 0.08)
	colInk = ui.Hex("#FFFFFF")
	colTermTop = ui.Hex("#F8FAFC")
	colTermBg = ui.Hex("#F8FAFC")
	colTermLine = ui.Hex("#E9EDF3")
	colTreeBg = ui.Hex("#F4F7FB")
	colFileLink = ui.Hex("#7C5CF0")
	colFilePerm = ui.Hex("#1D5FA8")
	colAccentText = ui.Hex("#FFFFFF")
	colSurfaceHover = ui.Hex("#EFF3F9")
	colSurfacePressed = ui.Hex("#E7EDF6")
	colAccentHover = ui.Hex("#2B64D9")
	colAccentPressed = ui.Hex("#2556C4")
	colScrollbar = ui.Hex("#CBD5E3")
}

func (a *App) uiDark() bool {
	switch a.settings.Theme {
	case 1:
		return true
	case 2:
		return false
	default:
		return a.systemDark
	}
}

func (a *App) themeSource() mygo.ThemeSource {
	switch a.settings.Theme {
	case 1:
		return mygo.ThemeDark
	case 2:
		return mygo.ThemeLight
	default:
		return mygo.ThemeSystem
	}
}

func (a *App) toggleTheme() {
	if a.uiDark() {
		a.setThemeMode(2)
	} else {
		a.setThemeMode(1)
	}
}

func (a *App) updatePalette(dark bool) {
	setPalette(dark)
	colCyan, colBlue, colAccentHover, colAccentPressed, colAccentSoft, colAccentLine =
		accentPalette(dark, a.settings.Accent)
}

func (a *App) setAccent(index int) {
	if index < 0 || index > 3 {
		return
	}
	a.settings.Accent = index
	a.updatePalette(a.uiDark())
	a.applyTermSettings()
	if a.store != nil {
		_ = a.store.SetSetting(a.ctx, "ui.accent", strconv.Itoa(index))
	}
	a.status = "强调色已更新"
}

func (a *App) setThemeMode(mode int) {
	if a.settings.Theme == mode {
		return
	}
	a.settings.Theme = mode
	a.updatePalette(a.uiDark())
	if mode == 2 {
		a.status = "已切换到浅色主题"
	} else {
		a.status = "已切换到深色主题"
	}
	a.applyTermSettings()
	a.ApplyThemeSource()
	if a.store != nil {
		_ = a.store.SetSetting(a.ctx, "terminal.theme", strconv.Itoa(a.settings.Theme))
	}
}

// ApplyThemeSource makes the native window controls and menus follow the
// app's saved theme. Call it after the window is attached.
func (a *App) ApplyThemeSource() {
	if a.win == nil {
		return
	}
	source := a.themeSource()
	mygo.Theme.SetSource(source)
	switch source {
	case mygo.ThemeDark:
		a.systemDark = true
	case mygo.ThemeLight:
		a.systemDark = false
	case mygo.ThemeSystem:
		a.systemDark = mygo.Theme.IsDark()
	}
	a.win.Invalidate()
}

// OnSystemAppearanceChanged refreshes the colors when the OS appearance
// changes while the app follows the system.
func (a *App) OnSystemAppearanceChanged() {
	if a.win == nil {
		return
	}
	a.post(func() {
		a.systemDark = mygo.Theme.IsDark()
	})
}

// accentPalette returns the design tokens for one of the four accent choices:
// accent, accent-2, hover, pressed, accent-soft and accent-line.
func accentPalette(dark bool, index int) (ui.Color, ui.Color, ui.Color, ui.Color, ui.Color, ui.Color) {
	if index < 0 || index > 3 {
		index = 0
	}
	if dark {
		switch index {
		case 1: // teal
			return ui.Hex("#2DD4BF"), ui.Hex("#38BDF8"), ui.Hex("#5EEAD4"), ui.Hex("#14B8A6"),
				ui.Hex("#2DD4BF").Alpha(0.13), ui.Hex("#2DD4BF").Alpha(0.42)
		case 2: // violet
			return ui.Hex("#A78BFA"), ui.Hex("#F0ABFC"), ui.Hex("#C4B5FD"), ui.Hex("#8B5CF6"),
				ui.Hex("#A78BFA").Alpha(0.14), ui.Hex("#A78BFA").Alpha(0.44)
		case 3: // orange
			return ui.Hex("#FB923C"), ui.Hex("#FBBF24"), ui.Hex("#FDBA74"), ui.Hex("#F97316"),
				ui.Hex("#FB923C").Alpha(0.14), ui.Hex("#FB923C").Alpha(0.44)
		default: // blue
			return ui.Hex("#4D8DFF"), ui.Hex("#38D6FF"), ui.Hex("#6BA3FF"), ui.Hex("#3C7BF0"),
				ui.Hex("#4D8DFF").Alpha(0.13), ui.Hex("#4D8DFF").Alpha(0.42)
		}
	}
	switch index {
	case 1: // teal
		return ui.Hex("#0D9488"), ui.Hex("#0891B2"), ui.Hex("#0F766E"), ui.Hex("#115E59"),
			ui.Hex("#0D9488").Alpha(0.09), ui.Hex("#0D9488").Alpha(0.35)
	case 2: // violet
		return ui.Hex("#7C3AED"), ui.Hex("#C026D3"), ui.Hex("#6D28D9"), ui.Hex("#5B21B6"),
			ui.Hex("#7C3AED").Alpha(0.09), ui.Hex("#7C3AED").Alpha(0.35)
	case 3: // orange
		return ui.Hex("#EA580C"), ui.Hex("#D97706"), ui.Hex("#C2410C"), ui.Hex("#9A3412"),
			ui.Hex("#EA580C").Alpha(0.09), ui.Hex("#EA580C").Alpha(0.35)
	default: // blue
		return ui.Hex("#2F6FEB"), ui.Hex("#0EA5E9"), ui.Hex("#2B64D9"), ui.Hex("#2556C4"),
			ui.Hex("#2F6FEB").Alpha(0.09), ui.Hex("#2F6FEB").Alpha(0.35)
	}
}

func (a *App) applyTheme(c *ui.Context) *ui.Theme {
	dark := a.uiDark()
	a.updatePalette(dark)

	base := *ui.DarkTheme()
	if !dark {
		base = *ui.LightTheme()
	}
	base.Background = colBg
	base.Surface = colPanel2
	base.SurfaceHover = colSurfaceHover
	base.SurfacePressed = colSurfacePressed
	base.Border = colLine
	base.Text = colText
	base.TextMuted = colMuted
	base.Accent = colCyan
	base.AccentHover = colAccentHover
	base.AccentPressed = colAccentPressed
	base.AccentText = colAccentText
	base.Danger = colRed
	base.Warning = colAmber
	base.Success = colGreen
	base.Selection = colAccentSoft
	base.Focus = colAccentLine
	base.Scrollbar = colScrollbar
	base.ScrollbarWidth = 8
	base.Radius = 8
	base.Spacing = 3
	base.FontSize = 11
	base.Font = "Segoe UI Variable Text"

	c.SetTheme(&base)
	return &base
}
