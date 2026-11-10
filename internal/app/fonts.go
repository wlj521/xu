package app

import (
	"slices"
	"strings"
	"sync"
)

const systemFontChoice = "系统默认（等宽）"

var (
	fontsOnce    sync.Once
	fontFamilies []string
)

// terminalFontChoices returns the system font choice followed by installed
// monospaced families, with the active custom family kept selectable even if
// the system no longer reports it.
func terminalFontChoices(current string) []string {
	fontsOnce.Do(func() {
		fontFamilies = cleanFontFamilies(systemFontFamilies())
	})

	fonts := fontFamilies
	current = strings.TrimSpace(current)
	if current != "" && !slices.Contains(fonts, current) {
		fonts = append(append([]string(nil), fonts...), current)
		sortFontFamilies(fonts)
	}

	choices := make([]string, 0, len(fonts)+1)
	choices = append(choices, systemFontChoice)
	return append(choices, fonts...)
}

func cleanFontFamilies(names []string) []string {
	seen := make(map[string]struct{}, len(names))
	fonts := make([]string, 0, len(names))
	for _, raw := range names {
		name := normalizeFontFamily(raw)
		if name == "" || strings.HasPrefix(name, "@") || !looksMonospaced(name) {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		fonts = append(fonts, name)
	}
	sortFontFamilies(fonts)
	return fonts
}

func looksMonospaced(name string) bool {
	lower := strings.ToLower(name)
	for _, hint := range []string{
		"mono", "code", "console", "courier", "fixed", "terminal",
		"typewriter", "consolas", "cascadia", "hack", "iosevka",
		"inconsolata", "meslo", "menlo", "monaco", "anonymous",
		"terminus", "profont", "proggy", "等宽", "等距", "编程", "更纱",
	} {
		if strings.Contains(lower, hint) {
			return true
		}
	}
	return false
}

func sortFontFamilies(fonts []string) {
	slices.SortFunc(fonts, func(a, b string) int {
		return strings.Compare(strings.ToLower(a), strings.ToLower(b))
	})
}

func normalizeFontFamily(name string) string {
	name = strings.TrimSpace(name)
	if i := strings.LastIndex(name, " ("); i > 0 && strings.HasSuffix(name, ")") {
		name = strings.TrimSpace(name[:i])
	}

	// Registry names include face styles; keep only the family so the menu
	// does not repeat Regular, Bold, Italic and their combinations.
	styles := []string{
		"bold italic", "bold oblique", "light italic", "medium italic",
		"semibold italic", "demibold italic", "extrabold italic",
		"extralight italic", "thin italic", "bold", "oblique", "italic",
		"regular", "light", "medium", "semibold", "demibold", "extrabold",
		"extralight", "thin", "condensed",
	}
	for {
		lower := strings.ToLower(name)
		changed := false
		for _, style := range styles {
			suffix := " " + style
			if strings.HasSuffix(lower, suffix) {
				name = strings.TrimSpace(name[:len(name)-len(suffix)])
				changed = true
				break
			}
		}
		if !changed {
			return name
		}
	}
}

func fontChoiceForFamily(family string) string {
	family = strings.TrimSpace(family)
	if family == "" {
		return systemFontChoice
	}
	return family
}

func fontFamilyForChoice(choice string) string {
	if choice == systemFontChoice {
		return ""
	}
	return strings.TrimSpace(choice)
}
