package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSettingsViewRenders(t *testing.T) {
	a := New()
	a.formOpen = false
	a.openSettings()

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()

	a.settingsSection = 0
	tt.Frame()
	for _, want := range []string{"设置", "主题", "强调色", "布局", "外观", "终端", "连接", "关于"} {
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
	a.settingsSection = 3

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
