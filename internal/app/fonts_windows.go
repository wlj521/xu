//go:build windows

package app

import "golang.org/x/sys/windows/registry"

const fontsRegistryPath = `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Fonts`

func systemFontFamilies() []string {
	var names []string
	for _, root := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
		key, err := registry.OpenKey(root, fontsRegistryPath, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		values, err := key.ReadValueNames(-1)
		_ = key.Close()
		if err == nil {
			names = append(names, values...)
		}
	}
	if len(names) == 0 {
		return fallbackFontFamilies
	}
	return names
}

var fallbackFontFamilies = []string{
	"Cascadia Code",
	"Cascadia Mono",
	"Consolas",
	"Courier New",
	"Lucida Console",
}
