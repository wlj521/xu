package app

import (
	"context"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"xu/internal/store"
)

func TestSettingsViewRenders(t *testing.T) {
	a := New()
	a.formOpen = false
	a.openSettings()

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()

	a.settingsSection = 0
	tt.Frame()
	for _, want := range []string{"设置", "主题", "强调色", "布局", "外观", "终端", "配色", "连接", "关于"} {
		if !tt.HasText(want) {
			t.Fatalf("appearance settings missing %q; texts = %q", want, tt.Texts())
		}
	}

	a.settingsSection = 1
	tt.Frame()
	for _, want := range []string{"字体", "终端字体", "字号", "回滚缓冲", "选中即复制"} {
		if !tt.HasText(want) {
			t.Fatalf("terminal settings missing %q; texts = %q", want, tt.Texts())
		}
	}
}

func TestSaveSettingsInMemory(t *testing.T) {
	a := New()
	a.formOpen = false
	a.openSettings()

	a.settingsFontSize = "16"
	a.settingsScrollback = "20"
	a.settings.Theme = 1
	a.saveSettings()

	if a.settingsErr != "" {
		t.Fatalf("save error: %s", a.settingsErr)
	}
	if a.settings.FontSize != 16 || a.settings.ScrollbackMB != 20 || a.settings.Theme != 1 {
		t.Fatalf("settings = %+v", a.settings)
	}

	d := a.termDisplay()
	if d.Font.Size != 16 || d.Scrollback != 20<<20 || d.Light != d.Dark {
		t.Fatalf("display = %+v", d)
	}
}

func TestAccentChoicesAndDefault(t *testing.T) {
	if got := defaultSettings().Accent; got != defaultAccent {
		t.Fatalf("default accent = %d, want %d (mint)", got, defaultAccent)
	}
	if len(accentChoices) < 8 {
		t.Fatalf("accent choices = %d, want the expanded palette", len(accentChoices))
	}
	if accentChoices[0].label != "薄荷绿" || accentChoices[0].index != defaultAccent {
		t.Fatalf("first accent = %+v, want mint as the default", accentChoices[0])
	}
	seen := map[int]bool{}
	for _, choice := range accentChoices {
		if seen[choice.index] {
			t.Fatalf("duplicate accent index %d", choice.index)
		}
		seen[choice.index] = true
		accent, second, hover, pressed, soft, line := accentPalette(true, choice.index)
		if accent == second || hover == pressed || soft.A == 0 || line.A == 0 {
			t.Fatalf("palette %d is incomplete", choice.index)
		}
	}
}

func TestTerminalSchemeSetting(t *testing.T) {
	names := terminal.GhosttyThemes()
	if len(names) == 0 {
		t.Skip("no Ghostty themes")
	}
	a := New()
	name := names[0]
	a.settings.TerminalScheme = name

	light, dark := a.termThemes()
	if light == nil || dark == nil || light != dark {
		t.Fatalf("scheme did not apply to both appearances: light=%p dark=%p", light, dark)
	}
	want, err := terminal.GhosttyTheme(name)
	if err != nil {
		t.Fatalf("load %q: %v", name, err)
	}
	if light.Background != want.Background || light.Foreground != want.Foreground {
		t.Fatalf("scheme colors = %+v, want %+v", light, want)
	}

	a.settings.TerminalScheme = "no such scheme"
	fallback, _ := a.termThemes()
	def, _ := a.defaultTermThemes()
	if fallback.Background != def.Background {
		t.Fatalf("an unknown scheme did not fall back to the app theme")
	}
}

func TestSchemeSyncsWorkspaceBackground(t *testing.T) {
	var name string
	var want *terminal.Theme
	for _, candidate := range terminal.GhosttyThemes() {
		th, err := terminal.GhosttyTheme(candidate)
		if err == nil && colorIsDark(th.Background) {
			name, want = candidate, th
			break
		}
	}
	if name == "" {
		t.Skip("no dark Ghostty theme")
	}

	a := New()
	a.settings.Theme = 1 // a dark window
	defer func() {
		a.settings.TerminalScheme = ""
		a.termDisplay()
	}()
	a.settings.TerminalScheme = name
	a.termDisplay()
	if colTermBg != want.Background {
		t.Fatalf("workspace background = %v, want the scheme's %v", colTermBg, want.Background)
	}

	// A light scheme stays out of the dark chrome, which returns to the
	// theme's own surface.
	lightName := ""
	var lightWant *terminal.Theme
	for _, candidate := range terminal.GhosttyThemes() {
		th, err := terminal.GhosttyTheme(candidate)
		if err == nil && !colorIsDark(th.Background) {
			lightName, lightWant = candidate, th
			break
		}
	}
	if lightName == "" {
		return
	}
	a.settings.TerminalScheme = ""
	a.termDisplay()
	defaultBg := colTermBg
	a.settings.TerminalScheme = lightName
	a.termDisplay()
	if colTermBg != defaultBg {
		t.Fatalf("a light scheme changed the dark workspace background")
	}
	if colTermBg == lightWant.Background {
		t.Fatalf("a light scheme was synced into the dark chrome")
	}
}

func TestTerminalSchemePersists(t *testing.T) {
	mygo.App.SetPath(mygo.PathUserData, t.TempDir())
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })

	st, err := store.Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	a := New()
	a.SetStore(st)
	a.settings.TerminalScheme = "Dracula"
	a.saveSettings()
	if a.settingsErr != "" {
		t.Fatalf("save settings: %s", a.settingsErr)
	}

	b := New()
	b.SetStore(st)
	if b.settings.TerminalScheme != "Dracula" {
		t.Fatalf("loaded scheme = %q, want Dracula", b.settings.TerminalScheme)
	}
}

func TestSettingsSchemeSection(t *testing.T) {
	a := New()
	a.openSettings()
	a.settingsSection = 2

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	for _, want := range []string{"配色", "预览", "Default", "主题文件夹", "重新加载"} {
		if !tt.HasText(want) {
			t.Fatalf("scheme section is missing %q; texts = %q", want, tt.Texts())
		}
	}
}

func TestSettingsSchemeAppliesOnClick(t *testing.T) {
	names := terminal.GhosttyThemes()
	if len(names) == 0 {
		t.Skip("no Ghostty themes")
	}
	a := New()
	a.openSettings()
	a.settingsSection = 2

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	name := names[0]
	if err := tt.Click(name); err != nil {
		t.Fatalf("click %q: %v", name, err)
	}
	tt.Frame()
	if a.settings.TerminalScheme != name {
		t.Fatalf("scheme = %q, want %q", a.settings.TerminalScheme, name)
	}

	if err := tt.Click("Default"); err != nil {
		t.Fatalf("click Default: %v", err)
	}
	tt.Frame()
	if a.settings.TerminalScheme != "" {
		t.Fatalf("scheme = %q, want the app default", a.settings.TerminalScheme)
	}
}

func TestSettingsShowsAllAccentLabels(t *testing.T) {
	a := New()
	a.openSettings()

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	for _, label := range []string{"薄荷绿", "蓝色", "紫色", "橙色", "玫瑰粉", "翡翠绿", "琥珀金", "靛蓝"} {
		if !tt.HasText(label) {
			t.Fatalf("settings is missing accent %q; texts = %q", label, tt.Texts())
		}
	}
}

func TestSettingsShortcutsSection(t *testing.T) {
	a := New()
	a.openSettings()
	a.settingsSection = 4

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	for _, want := range []string{"快捷键", "Ctrl Alt C", "Ctrl Shift F", "保存远程编辑"} {
		if !tt.HasText(want) {
			t.Fatalf("shortcuts section is missing %q; texts = %q", want, tt.Texts())
		}
	}
	if tt.HasText("GitHub") {
		t.Fatal("the shortcuts section must not carry the About content")
	}
}
