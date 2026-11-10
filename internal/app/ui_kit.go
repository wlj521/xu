package app

import "github.com/egoist/mygo/ui"

// Shared type sizes keep the native UI on the same scale as design/index.html.
const (
	fsCaption = 10
	fsSmall   = 11
	fsBody    = 12
	fsTitle   = 13
	fsDisplay = 20

	tabStripHeight = 36
	tabPillHeight  = 28
	tabPillRadius  = 7
)

func statusDot(c *ui.Context, color ui.Color, size float32) ui.Element {
	return ui.Box(c).Size(size, size).Radius(size / 2).Background(color).Shrink(0)
}

func styleTabPill(b ui.Element, active bool) ui.Element {
	b = b.Border(1, ui.Transparent)
	switch {
	case active:
		b.Background(colAccentSoft)
		b.Border(1, colAccentLine)
	case b.Hovered():
		b.Background(colPanel2.Alpha(0.72))
	}
	return b
}

func primaryAction(c *ui.Context, key, label string, icon *ui.SVG) ui.Element {
	b := ui.ButtonBase(c.Key(key)).Height(32).Padding(0, 14).Gap(7).
		Radius(6).AlignItems(ui.Center)
	b.Background(colCyan).Border(1, colCyan)
	if b.Hovered() {
		b.Background(colAccentHover).BorderColor(colAccentHover)
	}
	b.Children(func() {
		if icon != nil {
			iconEl(c, icon, 14, colAccentText)
		}
		ui.Text(c, label).FontSize(fsSmall).FontWeight(650).TextColor(colAccentText).NoWrap()
	})
	return b
}

func ghostAction(c *ui.Context, t *ui.Theme, key, label string, icon *ui.SVG) ui.Element {
	b := ui.ButtonBase(c.Key(key)).Height(30).Padding(0, 11).Gap(6).
		Radius(8).AlignItems(ui.Center)
	b.Background(colPanel2.Alpha(0.55)).Border(1, colLine)
	color := colMuted
	if b.Hovered() {
		b.Background(colPanel2)
		b.BorderColor(colLine2)
		color = colText
	}
	b.Children(func() {
		if icon != nil {
			iconEl(c, icon, 13, color)
		}
		ui.Text(c, label).FontSize(fsSmall).TextColor(color).NoWrap()
	})
	return b
}

func accentAction(c *ui.Context, key, label string, icon *ui.SVG) ui.Element {
	b := ui.ButtonBase(c.Key(key)).Height(28).Padding(0, 10).Gap(6).
		Radius(7).AlignItems(ui.Center)
	b.Background(colCyan.Alpha(0.09)).Border(1, colCyan.Alpha(0.2))
	if b.Hovered() {
		b.Background(colCyan.Alpha(0.15)).BorderColor(colCyan.Alpha(0.34))
	}
	b.Children(func() {
		if icon != nil {
			iconEl(c, icon, 12, colCyan)
		}
		ui.Text(c, label).FontSize(10.5).FontWeight(600).TextColor(colCyan).NoWrap()
	})
	return b
}

func quietAction(c *ui.Context, t *ui.Theme, key, label string, icon *ui.SVG, active bool) ui.Element {
	b := ui.ButtonBase(c.Key(key)).Height(26).Padding(0, 8).Gap(5).
		Radius(6).AlignItems(ui.Center)
	if active {
		b.Background(colPanel2.Alpha(0.9))
	} else if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	color := colMuted
	if active {
		color = colText
	}
	b.Children(func() {
		if icon != nil {
			iconEl(c, icon, 12, color)
		}
		ui.Text(c, label).FontSize(10).TextColor(color).NoWrap()
	})
	return b
}

func dangerAction(c *ui.Context, key, label string, icon *ui.SVG) ui.Element {
	b := ui.ButtonBase(c.Key(key)).Height(26).Padding(0, 8).Gap(5).
		Radius(6).AlignItems(ui.Center)
	if b.Hovered() {
		b.Background(colRed.Alpha(0.08))
	}
	b.Children(func() {
		if icon != nil {
			iconEl(c, icon, 12, colRed)
		}
		ui.Text(c, label).FontSize(10).TextColor(colRed).NoWrap()
	})
	return b
}

func accentIconButton(c *ui.Context, t *ui.Theme, key string, icon *ui.SVG, tip string) ui.Element {
	b := ui.ButtonBase(c.Key(key)).Size(30, 30).Radius(8).Center().Tooltip(tip).Label(tip)
	b.Background(colCyan.Alpha(0.07)).Border(1, colCyan.Alpha(0.22))
	if b.Hovered() {
		b.Background(colCyan.Alpha(0.12)).BorderColor(colCyan.Alpha(0.4))
	}
	b.Children(func() {
		color := colCyan
		if b.IsDisabled() {
			color = colMuted2
		}
		iconEl(c, icon, 15, color)
	})
	return b
}

func sectionLabel(c *ui.Context, label string) {
	ui.Text(c, label).FontSize(9.5).FontWeight(650).LetterSpacing(0.8).
		TextColor(colMuted2).NoWrap()
}

func miniTag(c *ui.Context, label string, color ui.Color) ui.Element {
	return ui.Box(c).Height(19).Padding(0, 6).Radius(3).
		Background(color.Alpha(0.07)).Border(1, color.Alpha(0.2)).Center().
		Children(func() {
			ui.Text(c, label).FontSize(8.5).FontWeight(500).TextColor(color).NoWrap()
		})
}

func pageHeader(c *ui.Context, t *ui.Theme, icon *ui.SVG, title, sub string, actions func()) {
	ui.Row(c).Gap(13).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(42, 42).Radius(9).
			Background(colCyan.Alpha(0.08)).Border(1, colCyan.Alpha(0.22)).Center().Children(func() {
			iconEl(c, icon, 19, colCyan)
		})
		ui.Column(c).Grow(1).Gap(3).Children(func() {
			ui.Text(c, title).FontSize(18).FontWeight(650)
			if sub != "" {
				ui.Text(c, sub).FontSize(10.5).TextColor(colMuted).NoWrap()
			}
		})
		if actions != nil {
			actions()
		}
	})
}

func cardPanel(c *ui.Context, build func()) {
	ui.Column(c).FillWidth().Padding(16).Background(colPanel).
		Border(1, colLine).Radius(8).Gap(12).Children(build)
}

func fieldColumn(c *ui.Context, label string, build func()) ui.Element {
	return ui.Column(c).Gap(5).Children(func() {
		ui.Text(c, label).FontSize(9.5).FontWeight(600).TextColor(colMuted)
		build()
	})
}

func hairline(c *ui.Context) {
	ui.Box(c).Height(1).FillWidth().Background(colLine)
}

// monitorWidth is the left-hand server monitor's width: narrower windows (the
// app runs at 200% DPI on small screens) give the workspace more room.
func monitorWidth(c *ui.Context) float32 {
	w, _ := c.Size()
	if w < 1000 {
		return 272
	}
	return 300
}

// fitDialog clamps a dialog size to the window, keeping a margin around it.
func fitDialog(c *ui.Context, w, h float32) (float32, float32) {
	cw, ch := c.Size()
	if maxW := cw - 40; w > maxW {
		w = maxW
	}
	if maxH := ch - 40; h > maxH {
		h = maxH
	}
	if w < 320 {
		w = 320
	}
	if h < 240 {
		h = 240
	}
	return w, h
}

// fitDialogWidth clamps only the width of a dialog whose height follows its
// content.
func fitDialogWidth(c *ui.Context, w float32) float32 {
	cw, _ := c.Size()
	if maxW := cw - 40; w > maxW {
		w = maxW
	}
	if w < 320 {
		w = 320
	}
	return w
}
