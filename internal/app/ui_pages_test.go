package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"xu/internal/store"
)

func TestRedesignedPagesRender(t *testing.T) {
	a := New()
	a.profiles = []store.Profile{{
		ID:       1,
		Name:     "alpha",
		Host:     "example.com",
		Port:     22,
		Username: "root",
	}}
	a.batchChoices = []batchChoice{{id: 1, name: "alpha", selected: true}}
	a.batchCmd = "uptime"
	a.terms = []*termSession{{ID: 1, Title: "alpha"}}
	a.activeTerm = 0
	a.forwardListen = "127.0.0.1:8080"
	a.forwardTarget = "127.0.0.1:80"

	cases := []struct {
		name string
		want string
		view func(*ui.Context, *ui.Theme)
	}{
		{"settings", "设置", a.settingsView},
		{"batch", "批量命令", a.batchView},
		{"forward", "端口转发", a.forwardView},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tt := ui.NewTester(func(c *ui.Context) {
				theme := a.applyTheme(c)
				tc.view(c, theme)
			}, 1280, 900)
			tt.Frame()
			if !tt.HasText(tc.want) {
				t.Fatalf("%s page does not show %q", tc.name, tc.want)
			}
		})
	}
}

func TestConnectionModalRenders(t *testing.T) {
	a := New()
	a.profiles = []store.Profile{{ID: 1, Name: "alpha", Host: "example.com", Port: 22, Username: "root"}}
	a.openConnectForm()

	tt := ui.NewTester(a.View, 1280, 900)
	tt.Frame()

	for _, want := range []string{"新建 SSH 连接", "名称", "主机", "端口", "用户名", "密码", "保存", "取消"} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q; texts = %q", want, tt.Texts())
		}
	}
}

func TestLightThemeApplies(t *testing.T) {
	a := New()
	a.settings.Theme = 2

	var theme *ui.Theme
	tt := ui.NewTester(func(c *ui.Context) {
		theme = a.applyTheme(c)
		a.settingsView(c, theme)
	}, 1280, 900)
	tt.Frame()

	if theme == nil || theme.Dark {
		t.Fatalf("expected a light theme, got %+v", theme)
	}
	if colBg != ui.Hex("#E9EDF4") {
		t.Fatalf("light palette was not applied: %v", colBg)
	}
}

func TestNewAndEditConnectionContexts(t *testing.T) {
	a := New()
	a.profileID = 7
	a.host = "old.example.com"
	a.openConnectForm()
	if a.profileID != 0 || a.host != "" || !a.formOpen {
		t.Fatalf("new connection did not reset form: id=%d host=%q open=%v", a.profileID, a.host, a.formOpen)
	}

	a.editProfile(store.Profile{
		ID:       2,
		Name:     "db-primary",
		Host:     "10.0.0.12",
		Port:     2222,
		Username: "deploy",
		AuthType: "password",
	})
	if !a.formOpen || a.profileID != 2 || a.host != "10.0.0.12" || a.port != "2222" {
		t.Fatalf("edit context = open:%v id:%d host:%q port:%q", a.formOpen, a.profileID, a.host, a.port)
	}
}

func TestFilePathBarShowsServerPath(t *testing.T) {
	a := New()
	b := &fileBrowser{
		remoteDir:  "/root",
		remoteHome: "/root",
	}

	tt := ui.NewTester(func(c *ui.Context) {
		theme := a.applyTheme(c)
		a.bottomPathBar(c, theme, b)
	}, 420, 60)
	tt.Frame()
	if !tt.HasText("/") || !tt.HasText("root") {
		t.Fatalf("breadcrumb path missing; texts = %q", tt.Texts())
	}
}

func TestMonitorCollapseButton(t *testing.T) {
	a := New()
	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	if err := tt.Click("收起监控"); err != nil {
		t.Fatalf("collapse: %v", err)
	}
	tt.Frame()
	if !a.monitorCollapsed {
		t.Fatal("monitor did not collapse")
	}
	if tt.HasText("收起监控") {
		t.Fatal("collapsed monitor should show its rail, not its header")
	}
	if err := tt.Click("展开监控"); err != nil {
		t.Fatalf("expand: %v", err)
	}
	tt.Frame()
	if a.monitorCollapsed {
		t.Fatal("monitor did not expand")
	}
}
