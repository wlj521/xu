package app

import (
	"os"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestChmodModeChecks(t *testing.T) {
	for _, mode := range []os.FileMode{0o644, 0o755, 0o600, 0} {
		if got := fileModeFromChecks(chmodChecks(mode)); got != mode {
			t.Errorf("chmod round trip %03o = %03o", mode, got)
		}
	}
}

func TestOpenChmodDialogUsesCurrentMode(t *testing.T) {
	a := New()
	b := newFileBrowser(1, nil, t.TempDir(), "")
	b.localEntries = []os.FileInfo{fakeInfo{name: "run.sh", mode: 0o640}}
	b.localSel = 0

	a.openChmodDialog(b, false)
	if !a.chmodOpen || a.chmodName != "run.sh" {
		t.Fatalf("chmod dialog = open:%v name:%q", a.chmodOpen, a.chmodName)
	}
	if got := fileModeFromChecks(a.chmodMode); got != 0o640 {
		t.Fatalf("chmod checkboxes start at %03o, want 640", got)
	}
}

func TestChmodModalRendersCheckboxes(t *testing.T) {
	a := New()
	a.chmodOpen = true
	a.chmodName = "appctl.sh"
	a.chmodMode = chmodChecks(0o755)

	tt := ui.NewTester(a.View, 1280, 900)
	tt.Frame()
	for _, want := range []string{
		"修改文件权限", "appctl.sh", "所有者", "组", "其他",
		"读取", "写入", "执行", "权限 755", "确定", "取消",
	} {
		if !tt.HasText(want) {
			t.Fatalf("chmod modal missing %q; texts = %q", want, tt.Texts())
		}
	}

	if err := tt.Click("读取"); err != nil {
		t.Fatalf("toggle owner read: %v", err)
	}
	tt.Frame()
	if a.chmodMode[0] {
		t.Fatal("owner read checkbox did not clear")
	}
	if err := tt.Click("取消"); err != nil {
		t.Fatalf("cancel chmod: %v", err)
	}
	tt.Frame()
	if a.chmodOpen {
		t.Fatal("cancel did not close the chmod modal")
	}
}
