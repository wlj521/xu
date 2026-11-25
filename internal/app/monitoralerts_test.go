package app

import (
	"context"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"xu/internal/monitorx"
	"xu/internal/sshx"
	"xu/internal/store"
)

func monitorTestSession() *termSession {
	return &termSession{
		ID: 1, Title: "alpha", profileID: 11,
		profile: sshx.Profile{Host: "alpha.example.com", Port: 22, Username: "root"},
	}
}

func TestMonitorThresholdAlertsOnlyNotifyOnTransitions(t *testing.T) {
	a := New()
	a.stop()
	ts := monitorTestSession()
	a.terms = []*termSession{ts}
	a.syncTabs()
	a.monitors[ts.ID] = &monitorState{termID: ts.ID}

	sample := monitorx.Sample{
		CPU: 95, MemUsed: 50, MemTotal: 100,
		DiskUsed: 50, DiskTotal: 100, LatencyMs: 20,
	}
	a.recordMonitorSample(ts, sample)
	if got := a.activeMonitorAlertCount(); got != 1 {
		t.Fatalf("active alerts = %d, want 1", got)
	}
	if len(a.pendingMonitorToasts) != 1 {
		t.Fatalf("toasts = %v", a.pendingMonitorToasts)
	}

	sample.CPU = 96
	a.recordMonitorSample(ts, sample)
	if got := a.activeMonitorAlertCount(); got != 1 {
		t.Fatalf("a repeated high sample changed active alerts to %d", got)
	}
	if len(a.pendingMonitorToasts) != 1 {
		t.Fatalf("a repeated high sample notified again: %v", a.pendingMonitorToasts)
	}

	sample.CPU = 80
	a.recordMonitorSample(ts, sample)
	if got := a.activeMonitorAlertCount(); got != 0 {
		t.Fatalf("alert did not clear below the release threshold: %d", got)
	}

	sample.CPU = 92
	a.recordMonitorSample(ts, sample)
	if got := a.activeMonitorAlertCount(); got != 1 {
		t.Fatalf("active alerts after a new crossing = %d, want 1", got)
	}
	if len(a.pendingMonitorToasts) != 2 {
		t.Fatalf("new crossing toasts = %v, want 2 total", a.pendingMonitorToasts)
	}
}

func TestMonitorDisconnectAlertOnlyOnStateChange(t *testing.T) {
	a := New()
	a.stop()
	ts := monitorTestSession()
	a.terms = []*termSession{ts}
	a.syncTabs()

	a.syncMonitorConnectionAlerts()
	ts.exited = true
	a.syncMonitorConnectionAlerts()
	if got := a.activeMonitorAlertCount(); got != 1 {
		t.Fatalf("disconnect alerts = %d, want 1", got)
	}
	if len(a.pendingMonitorToasts) != 1 {
		t.Fatalf("disconnect toasts = %v", a.pendingMonitorToasts)
	}

	a.syncMonitorConnectionAlerts()
	if got := a.activeMonitorAlertCount(); got != 1 || len(a.pendingMonitorToasts) != 1 {
		t.Fatalf("repeated disconnect state alerted again: alerts=%d toasts=%v",
			a.activeMonitorAlertCount(), a.pendingMonitorToasts)
	}

	ts.exited = false
	a.syncMonitorConnectionAlerts()
	if got := a.activeMonitorAlertCount(); got != 0 {
		t.Fatalf("reconnect did not clear the disconnect alert: %d", got)
	}
}

func TestMonitorAlertBannerOnlyShowsForActiveConnection(t *testing.T) {
	a := New()
	a.stop()
	a.profiles = []store.Profile{
		{ID: 11, Name: "alpha", Host: "alpha.example.com", Port: 22, Username: "root"},
		{ID: 22, Name: "beta", Host: "beta.example.com", Port: 2222, Username: "ops"},
	}
	alpha := monitorTestSession()
	beta := &termSession{
		ID: 2, Title: "beta", profileID: 22,
		profile: sshx.Profile{Host: "beta.example.com", Port: 2222, Username: "ops"},
	}
	a.terms = []*termSession{alpha, beta}
	a.syncTabs()
	a.syncMonitorConnectionAlerts()

	a.activateTabIndex(1)
	alpha.exited = true
	a.syncMonitorConnectionAlerts()
	a.pendingMonitorToasts = nil

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	if tt.HasText("1 个服务器告警") {
		t.Fatalf("an alert from another tab leaked into the active tab; texts = %q", tt.Texts())
	}

	a.activateTabIndex(0)
	tt.Frame()
	if !tt.HasText("1 个服务器告警") || !tt.HasText("SSH 连接已断开") {
		t.Fatalf("the disconnected tab lost its alert; texts = %q", tt.Texts())
	}
}

func TestMonitorAlertBannerKeepsSameServerTabsIndependent(t *testing.T) {
	a := New()
	a.stop()
	a.profiles = []store.Profile{
		{ID: 11, Name: "测试", Host: "test.example.com", Port: 22, Username: "root"},
	}
	first := monitorTestSession()
	first.ID = 1
	first.Title = "测试"
	second := monitorTestSession()
	second.ID = 2
	second.Title = "测试"
	a.terms = []*termSession{first, second}
	a.syncTabs()
	a.syncMonitorConnectionAlerts()

	second.exited = true
	a.syncMonitorConnectionAlerts()
	a.pendingMonitorToasts = nil

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	if tt.HasText("1 个服务器告警") {
		t.Fatalf("the disconnected duplicate tab leaked into the healthy tab; texts = %q", tt.Texts())
	}

	a.activateTabIndex(1)
	tt.Frame()
	if !tt.HasText("1 个服务器告警") || !tt.HasText("SSH 连接已断开") {
		t.Fatalf("the disconnected tab lost its own alert; texts = %q", tt.Texts())
	}
}

func TestMonitorDisconnectClearsStaleThresholdAlerts(t *testing.T) {
	a := New()
	a.stop()
	ts := monitorTestSession()
	a.terms = []*termSession{ts}
	a.syncTabs()
	a.monitors[ts.ID] = &monitorState{termID: ts.ID}
	a.syncMonitorConnectionAlerts()
	a.recordMonitorSample(ts, monitorx.Sample{
		CPU: 95, MemUsed: 50, MemTotal: 100,
		DiskUsed: 50, DiskTotal: 100, LatencyMs: 20,
	})
	if got := a.activeMonitorAlertCount(); got != 1 {
		t.Fatalf("threshold alerts = %d, want 1", got)
	}

	ts.exited = true
	a.syncMonitorConnectionAlerts()
	active := a.activeMonitorAlertsForTerm(ts.ID)
	if len(active) != 1 || active[0].Kind != monitorAlertDisconnect {
		t.Fatalf("active alerts after disconnect = %+v", active)
	}
}

func TestServerOverviewShowsAllConnectedServers(t *testing.T) {
	a := New()
	a.stop()
	a.profiles = []store.Profile{
		{ID: 11, Name: "alpha", Host: "alpha.example.com", Port: 22, Username: "root"},
		{ID: 22, Name: "beta", Host: "beta.example.com", Port: 2222, Username: "ops"},
	}
	alpha := monitorTestSession()
	beta := &termSession{
		ID: 2, Title: "beta", profileID: 22,
		profile: sshx.Profile{Host: "beta.example.com", Port: 2222, Username: "ops"},
	}
	a.terms = []*termSession{alpha, beta}
	a.syncTabs()
	a.monitors[alpha.ID] = &monitorState{termID: alpha.ID}
	a.monitors[beta.ID] = &monitorState{termID: beta.ID}
	a.recordMonitorSample(alpha, monitorx.Sample{
		At: time.Now(), CPU: 95, MemUsed: 50, MemTotal: 100,
		DiskUsed: 50, DiskTotal: 100, LatencyMs: 20,
	})
	a.recordMonitorSample(beta, monitorx.Sample{
		At: time.Now(), CPU: 12, MemUsed: 30, MemTotal: 100,
		DiskUsed: 40, DiskTotal: 100, LatencyMs: 35,
	})

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	if _, ok := tt.Find("服务器总览"); !ok {
		t.Fatalf("the left monitor lost its overview action; texts = %q", tt.Texts())
	}
	if _, ok := tt.Find("服务器总览与告警"); ok {
		t.Fatal("the title bar must not show a server overview action")
	}
	if !tt.HasText("1 个服务器告警") || !tt.HasText("查看总览") {
		t.Fatalf("alert banner missing; texts = %q", tt.Texts())
	}

	a.openServerOverview()
	tt.Frame()
	for _, want := range []string{
		"服务器总览与告警", "alpha", "beta", "CPU", "内存", "磁盘", "延迟",
		"1 台告警", "CPU 95%",
	} {
		if !tt.HasText(want) {
			t.Fatalf("overview missing %q; texts = %q", want, tt.Texts())
		}
	}
	if err := tt.Click("查看 beta"); err != nil {
		t.Fatal(err)
	}
	if a.overviewOpen || a.activeTab != 1 || a.activeTerm != 1 {
		t.Fatalf("overview row did not focus beta: open=%v activeTab=%d activeTerm=%d",
			a.overviewOpen, a.activeTab, a.activeTerm)
	}
	if !a.monitorOpen {
		t.Fatal("overview row did not open the server monitor")
	}
	tt.Frame()
	if !tt.HasText("服务器监控") || !tt.HasText("CPU 使用率") {
		t.Fatalf("monitor detail did not render after 查看; texts = %q", tt.Texts())
	}
	if _, ok := tt.Find("返回终端"); ok {
		t.Fatalf("the monitor page must not carry a terminal button; texts = %q", tt.Texts())
	}
	if _, ok := tt.Find("文件"); ok {
		t.Fatalf("the monitor page must not carry a files button; texts = %q", tt.Texts())
	}
	if !tt.HasText("按 Esc 返回终端") {
		t.Fatalf("monitor page does not name its way back; texts = %q", tt.Texts())
	}
	if err := tt.Click("关闭监控（Esc）"); err != nil {
		t.Fatal(err)
	}
	if a.monitorOpen {
		t.Fatal("the close button did not close the monitor page")
	}
	tt.Frame()
	if tt.HasText("按 Esc 返回终端") {
		t.Fatal("monitor page stayed after the close button")
	}
}

func TestMonitorPageClosesWithEscapeAndCloseButton(t *testing.T) {
	a := New()
	a.stop()
	ts := monitorTestSession()
	a.terms = []*termSession{ts}
	a.syncTabs()
	a.monitors[ts.ID] = &monitorState{termID: ts.ID}
	a.recordMonitorSample(ts, monitorx.Sample{
		CPU: 12, MemUsed: 30, MemTotal: 100,
		DiskUsed: 40, DiskTotal: 100, LatencyMs: 20,
	})
	a.monitorOpen = true

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	if _, ok := tt.Find("关闭监控（Esc）"); !ok {
		t.Fatalf("monitor close button missing; texts = %q", tt.Texts())
	}
	if err := tt.Click("关闭监控（Esc）"); err != nil {
		t.Fatal(err)
	}
	if a.monitorOpen {
		t.Fatal("close button did not leave the monitor page")
	}

	a.monitorOpen = true
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	if a.monitorOpen {
		t.Fatal("Escape did not leave the monitor page")
	}
}

func TestMonitorAlertSettingsPersist(t *testing.T) {
	mygo.App.SetPath(mygo.PathUserData, t.TempDir())
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })

	st, err := store.Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	a := New()
	a.SetStore(st)
	a.settings.MonitorAlerts = false
	a.settings.MonitorDisconnect = false
	a.settings.MonitorDesktop = false
	a.monitorCPUPick = "83"
	a.monitorMemPick = "71"
	a.monitorDiskPick = "96"
	a.monitorLatencyPick = "237"
	a.saveSettings()
	if a.settingsErr != "" {
		t.Fatalf("save settings: %s", a.settingsErr)
	}

	b := New()
	b.SetStore(st)
	if b.settings.MonitorAlerts || b.settings.MonitorDisconnect || b.settings.MonitorDesktop {
		t.Fatalf("monitor switches did not persist: %+v", b.settings)
	}
	if b.settings.MonitorCPU != 83 || b.settings.MonitorMemory != 71 ||
		b.settings.MonitorDisk != 96 || b.settings.MonitorLatency != 237 {
		t.Fatalf("monitor thresholds did not persist: %+v", b.settings)
	}
}

func TestMonitorThresholdInputValidation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		check func(string) (int, bool)
	}{
		{name: "percent minimum", value: "50%", check: monitorPercentValue},
		{name: "percent maximum", value: "100", check: monitorPercentValue},
		{name: "latency minimum", value: "50", check: monitorLatencyValue},
		{name: "latency maximum", value: "5000 ms", check: monitorLatencyValue},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := tc.check(tc.value); !ok {
				t.Fatalf("%q should be accepted", tc.value)
			}
		})
	}

	a := New()
	a.stop()
	a.monitorCPUPick = "49"
	a.saveSettings()
	if a.settingsErr != "CPU 阈值需为 50–100 的整数" {
		t.Fatalf("invalid CPU threshold error = %q", a.settingsErr)
	}
	if a.settings.MonitorCPU != 90 {
		t.Fatalf("invalid input changed the saved CPU threshold to %d", a.settings.MonitorCPU)
	}
}
