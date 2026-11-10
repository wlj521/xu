package app

import "github.com/egoist/mygo/ui"

func svgIcon(body string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">` + body + `</svg>`))
}

func fileDocIcon(detail string) *ui.SVG {
	return svgIcon(`<path d="M5.6 2.9h7.2l5.2 5.2v11.4a1.8 1.8 0 0 1-1.8 1.8H7.4a1.8 1.8 0 0 1-1.8-1.8V4.7a1.8 1.8 0 0 1 1.8-1.8Z"/><path d="M12.8 2.9v5.2h5.2"/>` + detail)
}

var (
	iconServer       = svgIcon(`<rect x="3.5" y="4" width="17" height="6" rx="1.6"/><rect x="3.5" y="14" width="17" height="6" rx="1.6"/><path d="M7 7h.01M7 17h.01M11 7h5M11 17h5"/>`)
	iconTerm         = svgIcon(`<rect x="3" y="4.5" width="18" height="15" rx="2.5"/><path d="m7 9 3 3-3 3M12.5 15H17"/>`)
	iconFolder       = svgIcon(`<path d="M3.1 8h5.8l2 2h9.9l-1.3 7.8a2 2 0 0 1-2 1.7H5.2a2 2 0 0 1-2-1.8L2.4 9.2A1.4 1.4 0 0 1 3.1 8Z"/><path d="M3.1 8V6.2a1.7 1.7 0 0 1 1.7-1.7h4.4l2 2h6.9a1.7 1.7 0 0 1 1.7 1.7V8"/>`)
	iconFileFolder   = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path d="M2.7 7.2C2.7 6.1 3.6 5.2 4.7 5.2h5.1l2.1 2.1h7.4c1.1 0 2 .9 2 2v8c0 1.1-.9 2-2 2H4.7c-1.1 0-2-.9-2-2V7.2Z" fill="#F8CC5B" stroke="#D8A53A" stroke-width="1.2" stroke-linejoin="round"/><path d="M2.7 9.15h18.6" fill="none" stroke="#D8A53A" stroke-width="1.1"/><path d="M4.9 6.45h4.1" fill="none" stroke="#FFE39B" stroke-width="1.1" stroke-linecap="round"/></svg>`))
	iconFile         = fileDocIcon("")
	iconFileText     = fileDocIcon(`<path d="M8.3 12.4h7.2M8.3 15.1h7.2M8.3 17.8h4.7"/>`)
	iconFileScript   = fileDocIcon(`<path d="m8.5 12.3 2.2 2.1-2.2 2.1M12.6 16.5h3.2"/>`)
	iconFileCode     = fileDocIcon(`<path d="m10.2 12.4-2 2.1 2 2.1M13.8 12.4l2 2.1-2 2.1"/>`)
	iconFileConfig   = fileDocIcon(`<path d="M8.3 12.5h7.2M8.3 15.3h7.2M8.3 18.1h7.2"/><circle cx="10.7" cy="12.5" r="1.05" fill="currentColor" stroke="none"/><circle cx="13.2" cy="15.3" r="1.05" fill="currentColor" stroke="none"/><circle cx="11.6" cy="18.1" r="1.05" fill="currentColor" stroke="none"/>`)
	iconFileImage    = fileDocIcon(`<circle cx="15.2" cy="12.7" r="1.1"/><path d="m7.9 18.6 2.8-3.1 1.8 1.7 2.1-2.6 2.5 4"/>`)
	iconFileTable    = fileDocIcon(`<rect x="8.2" y="12.2" width="7.6" height="6.4" rx=".7"/><path d="M8.2 14.3h7.6M8.2 16.4h7.6M12 12.2v6.4"/>`)
	iconFileMedia    = fileDocIcon(`<path d="m10.2 12.5 4.2 2.5-4.2 2.5v-5Z" fill="currentColor" stroke="none"/>`)
	iconFileArchive  = svgIcon(`<path d="m4.1 7.5 7.9-3.8 7.9 3.8v9L12 20.3l-7.9-3.8v-9Z"/><path d="m4.1 7.5 7.9 3.9 7.9-3.9M12 11.4v8.9"/><path d="M9.7 5.1v2.2M14.3 5.1v2.2M10.5 13.2h3M10.5 15.3h3"/>`)
	iconLink         = svgIcon(`<path d="M10 13a5 5 0 0 0 7.1 0l2-2a5 5 0 0 0-7.1-7.1l-1.1 1.1M14 11a5 5 0 0 0-7.1 0l-2 2A5 5 0 0 0 12 20.1l1.1-1.1"/>`)
	iconSearch       = svgIcon(`<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>`)
	iconGear         = svgIcon(`<circle cx="12" cy="12" r="3.2"/><path d="M19.4 15a1.7 1.7 0 0 0 .34 1.88l.06.06-2.83 2.83-.06-.06a1.7 1.7 0 0 0-1.88-.34 1.7 1.7 0 0 0-1.03 1.56V21h-4v-.08A1.7 1.7 0 0 0 8.94 19.4a1.7 1.7 0 0 0-1.88.34l-.06.06-2.83-2.83.06-.06A1.7 1.7 0 0 0 4.57 15a1.7 1.7 0 0 0-1.56-1.03H3v-4h.08A1.7 1.7 0 0 0 4.6 8.94a1.7 1.7 0 0 0-.34-1.88L4.2 7l2.83-2.83.06.06A1.7 1.7 0 0 0 9 4.57a1.7 1.7 0 0 0 1.03-1.56V3h4v.08A1.7 1.7 0 0 0 15.06 4.6a1.7 1.7 0 0 0 1.88-.34L17 4.2 19.83 7l-.06.06A1.7 1.7 0 0 0 19.43 9c.2.6.77 1 1.4 1h.17v4h-.08A1.7 1.7 0 0 0 19.4 15Z"/>`)
	iconPanel        = svgIcon(`<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M9 4v16"/>`)
	iconLayoutH      = svgIcon(`<rect x="3.5" y="4.5" width="17" height="15" rx="2"/><path d="M3.5 9.5h17"/>`)
	iconLayoutV      = svgIcon(`<rect x="3.5" y="4.5" width="17" height="15" rx="2"/><path d="M9.5 4.5v15"/>`)
	iconSun          = svgIcon(`<circle cx="12" cy="12" r="4"/><path d="M12 2.5v2M12 19.5v2M2.5 12h2M19.5 12h2M5.2 5.2l1.4 1.4M17.4 17.4l1.4 1.4M18.8 5.2l-1.4 1.4M6.6 17.4l-1.4 1.4"/>`)
	iconHelp         = svgIcon(`<circle cx="12" cy="12" r="8.5"/><path d="M9.6 9.2a2.5 2.5 0 0 1 4.9.7c0 1.8-2.5 2.1-2.5 3.6M12 17h.01"/>`)
	iconShield       = svgIcon(`<path d="M12 3 5 6v5.5c0 4.3 3 7.4 7 9.5 4-2.1 7-5.2 7-9.5V6l-7-3Z"/><path d="m9 12 2 2 4-4"/>`)
	iconLock         = svgIcon(`<rect x="4.5" y="10" width="15" height="10.5" rx="2.2"/><path d="M7.8 10V7.6a4.2 4.2 0 0 1 8.4 0V10M12 14v2.6"/>`)
	iconMoon         = svgIcon(`<path d="M20 14.2A8.2 8.2 0 0 1 9.8 4 8.4 8.4 0 1 0 20 14.2Z"/>`)
	iconMore         = svgIcon(`<circle cx="5" cy="12" r="1.4" fill="currentColor" stroke="none"/><circle cx="12" cy="12" r="1.4" fill="currentColor" stroke="none"/><circle cx="19" cy="12" r="1.4" fill="currentColor" stroke="none"/>`)
	iconPlus         = svgIcon(`<path d="M12 5v14M5 12h14"/>`)
	iconClose        = svgIcon(`<path d="m6 6 12 12M18 6 6 18"/>`)
	iconRefresh      = svgIcon(`<path d="M20 11a8 8 0 1 0-2.3 5.7M20 5v6h-6"/>`)
	iconUpload       = svgIcon(`<path d="M12 16V4m0 0L7 9m5-5 5 5M4 16v3.5A1.5 1.5 0 0 0 5.5 21h13a1.5 1.5 0 0 0 1.5-1.5V16"/>`)
	iconDownload     = svgIcon(`<path d="M12 4v12m0 0 5-5m-5 5-5-5M4 16v3.5A1.5 1.5 0 0 0 5.5 21h13a1.5 1.5 0 0 0 1.5-1.5V16"/>`)
	iconNewFolder    = svgIcon(`<path d="M3.5 7.5h6l2 2h9v9.5a1.5 1.5 0 0 1-1.5 1.5h-14A1.5 1.5 0 0 1 3.5 19V7.5Z"/><path d="M12 12v5M9.5 14.5h5"/>`)
	iconFolderUp     = svgIcon(`<path d="M3.5 7.5h6l2 2h9v9.5a1.5 1.5 0 0 1-1.5 1.5h-14A1.5 1.5 0 0 1 3.5 19V7.5Z"/><path d="M12 17v-5m0 0-2.2 2.2M12 12l2.2 2.2"/>`)
	iconChevron      = svgIcon(`<path d="m6 9 6 6 6-6"/>`)
	iconChevronRight = svgIcon(`<path d="m9 6 6 6-6 6"/>`)
	iconChevronLeft  = svgIcon(`<path d="m15 6-6 6 6 6"/>`)
	iconArrowUp      = svgIcon(`<path d="M12 19V5M5 12l7-7 7 7"/>`)
	iconActivity     = svgIcon(`<path d="M3 12h4l2.5-7 5 14L17 12h4"/>`)
	iconHistory      = svgIcon(`<path d="M4 12a8 8 0 1 0 2.4-5.7M4 5v4h4M12 8v4l3 2"/>`)
	iconCommand      = svgIcon(`<path d="M9 6a3 3 0 1 1 3 3v6a3 3 0 1 1-3-3h6a3 3 0 1 1-3 3V9a3 3 0 1 1 3 3H9a3 3 0 1 1 3-3"/>`)
	iconCopy         = svgIcon(`<rect x="8" y="8" width="11" height="12" rx="1.6"/><path d="M16 8V6.5A1.5 1.5 0 0 0 14.5 5h-9A1.5 1.5 0 0 0 4 6.5v10A1.5 1.5 0 0 0 5.5 18H8"/>`)
	iconExpand       = svgIcon(`<path d="M9 4H4v5M15 4h5v5M15 20h5v-5M9 20H4v-5"/>`)
	iconChevronUp    = svgIcon(`<path d="m6 15 6-6 6 6"/>`)
	iconSave         = svgIcon(`<path d="M5 3.5h10.5L20 8v12.5a1.5 1.5 0 0 1-1.5 1.5h-13A1.5 1.5 0 0 1 4 20.5v-15A1.5 1.5 0 0 1 5.5 3.5Z"/><path d="M8 3.5v5h7v-5M8 14h8v7H8z"/>`)
	iconEdit         = svgIcon(`<path d="M4 20h4l10.5-10.5a2.1 2.1 0 0 0-3-3L5 17v3Z"/><path d="m13.5 8.5 3 3"/>`)
	iconTrash        = svgIcon(`<path d="M4 7h16M9 7V4.5h6V7M6.5 7l1 13h9l1-13M10 11v5M14 11v5"/>`)
	iconCheck        = svgIcon(`<path d="m4.5 12.5 5 5L19.5 7"/>`)
	iconEraser       = svgIcon(`<path d="m4 15 5 5h11M20 10 10 20l-6-6L14 4l6 6Z"/>`)
	iconPower        = svgIcon(`<path d="M12 3v9M6.3 6.3a8 8 0 1 0 11.4 0"/>`)
	iconConnections  = svgIcon(`<path d="M3.5 7.5h6l2 2h9v9.5a1.5 1.5 0 0 1-1.5 1.5h-14A1.5 1.5 0 0 1 3.5 19V7.5Z"/><path d="M8 13h8M8 16h5"/>`)
	iconManager      = svgIcon(`<path d="M3.5 8.2h5.9l2 2h9.1v8a2 2 0 0 1-2 2H5.5a2 2 0 0 1-2-2V8.2Z"/><path d="M3.5 8.2V6.4a1.7 1.7 0 0 1 1.7-1.7h4.4l2 2h6.7a1.7 1.7 0 0 1 1.7 1.7v1.8"/><path d="M7.6 12.6h7.8M7.6 15.6h5.2"/>`)
)

func iconEl(c *ui.Context, s *ui.SVG, size float32, color ui.Color) ui.Element {
	return ui.Icon(c, s).Size(size, size).TextColor(color)
}

func iconButton(c *ui.Context, t *ui.Theme, key string, s *ui.SVG, tip string) ui.Element {
	b := ui.ButtonBase(c.Key(key)).Size(30, 30).Radius(8).Center().Tooltip(tip).Label(tip)
	color := t.TextMuted
	if b.Hovered() {
		b.Background(t.SurfaceHover)
		color = t.Text
	}
	b.Children(func() { iconEl(c, s, 15, color) })
	return b
}
