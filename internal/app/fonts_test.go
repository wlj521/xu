package app

import (
	"slices"
	"testing"
)

func TestNormalizeFontFamily(t *testing.T) {
	cases := map[string]string{
		"Cascadia Code Regular (TrueType)":             "Cascadia Code",
		"JetBrains Mono NL SemiBold Italic (TrueType)": "JetBrains Mono NL",
		"Ubuntu Sans Mono Regular (TrueType)":          "Ubuntu Sans Mono",
		"Arial Bold Italic (TrueType)":                 "Arial",
		"Arial Black (TrueType)":                       "Arial Black",
		"Microsoft YaHei UI (TrueType)":                "Microsoft YaHei UI",
		"@MS Gothic (TrueType)":                        "@MS Gothic",
	}
	for in, want := range cases {
		if got := normalizeFontFamily(in); got != want {
			t.Errorf("normalizeFontFamily(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTerminalFontChoicesKeepCurrentFamily(t *testing.T) {
	choices := terminalFontChoices("Missing Terminal Font")
	if len(choices) == 0 || choices[0] != systemFontChoice {
		t.Fatalf("font choices must start with %q: %v", systemFontChoice, choices)
	}
	if !slices.Contains(choices, "Missing Terminal Font") {
		t.Fatalf("current font missing from choices: %v", choices)
	}
}

func TestLooksMonospaced(t *testing.T) {
	for _, name := range []string{
		"Cascadia Code",
		"JetBrains Mono",
		"Courier New",
		"Lucida Sans Typewriter",
		"等距更纱黑体 SC",
	} {
		if !looksMonospaced(name) {
			t.Errorf("%q should be treated as monospaced", name)
		}
	}
	for _, name := range []string{"Arial", "Microsoft YaHei UI", "Times New Roman"} {
		if looksMonospaced(name) {
			t.Errorf("%q should not be treated as monospaced", name)
		}
	}
}

func TestFontChoiceMapping(t *testing.T) {
	if got := fontChoiceForFamily("  "); got != systemFontChoice {
		t.Fatalf("empty family choice = %q", got)
	}
	if got := fontFamilyForChoice(systemFontChoice); got != "" {
		t.Fatalf("system choice family = %q", got)
	}
	if got := fontChoiceForFamily(" Cascadia Code "); got != "Cascadia Code" {
		t.Fatalf("custom family choice = %q", got)
	}
}
