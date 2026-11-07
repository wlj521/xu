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
	for _, want := range []string{"字体", "字号", "回滚缓冲", "选中即复制"} {
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
