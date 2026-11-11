package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"xu/internal/monitorx"
	"xu/internal/sshx"
)

func TestInspectorRendersSSH(t *testing.T) {
	a := New()
	a.formOpen = false
	a.bottomFiles = false
	a.terms = []*termSession{{
		ID:    1,
		Title: "root@test",
		profile: sshx.Profile{
			Host:     "test",
			Port:     22,
			Username: "root",
		},
	}}
	a.activeTerm = 0

	now := time.Now()
	samples := make([]monitorx.Sample, 0, 5)
	for i := 0; i < 5; i++ {
		samples = append(samples, monitorx.Sample{
			At:        now.Add(time.Duration(i) * 2 * time.Second),
			CPU:       float64(12 + i*3),
			MemUsed:   700 << 20,
			MemTotal:  1 << 30,
			SwapUsed:  20 << 20,
			SwapTotal: 512 << 20,
			DiskUsed:  10 << 30,
			DiskTotal: 40 << 30,
			Disks: []monitorx.Disk{
				{Mount: "/", Used: 10 << 30, Total: 40 << 30},
				{Mount: "/boot", Used: 500 << 20, Total: 1 << 30},
			},
			RxRate:    1024 * float64(i+1),
			TxRate:    512 * float64(i+1),
			LatencyMs: 14,
			Interfaces: []monitorx.Interface{
				{Name: "eth0", RxRate: 1024 * float64(i+1), TxRate: 512 * float64(i+1)},
			},
			Load1:     0.5,
			CPUs:      4,
			OS:        "Ubuntu 22.04.4 LTS",
			Kernel:    "Linux 5.15.0-91-generic",
			UptimeSec: 35444321,
			Processes: []monitorx.Process{
				{Name: "java", RSS: 1800 << 20, CPU: 11.3},
				{Name: "mysqld", RSS: 113 << 20, CPU: 3.7},
			},
		})
	}
	a.monitors[1] = &monitorState{termID: 1, samples: samples}

	tt := ui.NewTester(a.View, 1280, 800)
	tt.Frame()
	for _, want := range []string{
		"CPU 使用率", "内存", "交换", "网络", "全部网卡", "延迟 · PING", "本机 · 14 ms",
		"磁盘", "/boot", "进程", "主机", "java", "11.3%",
	} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q; texts = %q", want, tt.Texts())
		}
	}

	if err := tt.Click("选择网卡"); err != nil {
		t.Fatalf("open the interface picker: %v", err)
	}
	tt.Frame()
	if err := tt.ChooseMenuItem("eth0"); err != nil {
		t.Fatalf("choose eth0: %v", err)
	}
	tt.Frame()
	if a.netIface[1] != "eth0" || !tt.HasText("eth0") {
		t.Fatalf("interface selection = %q, texts = %q", a.netIface[1], tt.Texts())
	}
	for _, gone := range []string{"概览", "连接信息"} {
		if tt.HasText(gone) {
			t.Fatalf("inspector still has obsolete tab %q", gone)
		}
	}
}

func TestLocalInspectorOffersConnectionManager(t *testing.T) {
	a := New()
	a.stop()
	a.terms = []*termSession{{ID: 1, Title: "PowerShell 7", local: true}}
	a.syncTabs()

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	if !tt.HasText("连接管理") {
		t.Fatalf("local monitor has no connection manager action; texts = %q", tt.Texts())
	}
	if err := tt.Click("连接管理"); err != nil {
		t.Fatalf("click 连接管理: %v", err)
	}
	if !a.managerOpen {
		t.Fatal("the local monitor action must open the connection manager")
	}
}

func TestSettingsAboutShowsGitHub(t *testing.T) {
	a := New()
	a.openSettings()
	a.settingsSection = 4

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	for _, want := range []string{"GitHub", "在浏览器打开 GitHub", "检查更新"} {
		if !tt.HasText(want) {
			t.Fatalf("about section is missing %q; texts = %q", want, tt.Texts())
		}
	}
	if tt.HasText(githubURL) {
		t.Fatalf("about section should not print the URL; texts = %q", tt.Texts())
	}
}

func TestRateBucketsKeepsPeaks(t *testing.T) {
	rx, tx := rateBuckets([]float64{1, 3, 2, 0}, []float64{2, 1, 5, 4}, 2)
	if rx[0] != 3 || tx[0] != 2 {
		t.Fatalf("first bucket = %v/%v, want the peaks 3/2", rx[0], tx[0])
	}
	if rx[1] != 2 || tx[1] != 5 {
		t.Fatalf("second bucket = %v/%v, want the peaks 2/5", rx[1], tx[1])
	}
}
