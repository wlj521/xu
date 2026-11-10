package app

import (
	_ "embed"

	"github.com/egoist/mygo/ui"
)

//go:embed title-icon.png
var titleIconPNG []byte

// titleIcon is the same artwork as the application icon, rendered here so
// the in-app brand mark and the exe/taskbar icon never drift apart.
var titleIcon = mustDecodeTitleIcon()

func mustDecodeTitleIcon() *ui.Bitmap {
	icon, err := ui.DecodeBitmap(titleIconPNG)
	if err != nil {
		panic("decode title icon: " + err.Error())
	}
	return icon
}
