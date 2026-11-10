//go:build !windows

package app

var fallbackFontFamilies = []string{
	"SF Mono",
	"Menlo",
	"Monaco",
	"DejaVu Sans Mono",
	"Liberation Mono",
	"Noto Sans Mono",
}

func systemFontFamilies() []string {
	return fallbackFontFamilies
}
