package app

import (
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestUpdateModalShowsFriendlyError(t *testing.T) {
	a := New()
	a.updateOpen = true
	a.updateErr = "更新源暂未就绪：GitHub 最新版本还没有附带更新清单，请先发布带自动更新的版本。"
	a.updateDetail = "GET https://github.com/wlj521/xu/releases/latest/download/update-windows-amd64.json: 404 Not Found"

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	for _, want := range []string{"检查更新", "更新源暂未就绪", "重试检查"} {
		if !tt.HasText(want) {
			t.Fatalf("update dialog is missing %q; texts = %q", want, tt.Texts())
		}
	}
}

func TestUpdateModalShowsAvailableVersion(t *testing.T) {
	a := New()
	a.updateOpen = true
	a.updateAvailable = &mygo.Update{Version: "9.9.9", Notes: "- 新功能\n- 修复问题"}

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	for _, want := range []string{"发现新版本 Xu 9.9.9", "立即更新", "稍后"} {
		if !tt.HasText(want) {
			t.Fatalf("update dialog is missing %q; texts = %q", want, tt.Texts())
		}
	}
}
