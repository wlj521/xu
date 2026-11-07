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
			RxRate:    1024 * float64(i+1),
			TxRate:    512 * float64(i+1),
			Load1:     0.5,
			CPUs:      4,
			OS:        "Ubuntu 22.04.4 LTS",
			Kernel:    "Linux 5.15.0-91-generic",
			UptimeSec: 35444321,
			Processes: []monitorx.Process{
				{Name: "java", RSS: 1800 << 20},
				{Name: "mysqld", RSS: 113 << 20},
			},
		})
	}
	a.monitors[1] = &monitorState{termID: 1, samples: samples}

	tt := ui.NewTester(a.View, 1280, 800)
	tt.Frame()
	for _, want := range []string{"服务器监控", "CPU 使用率", "内存", "网络", "磁盘", "进程", "主机", "java"} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q; texts = %q", want, tt.Texts())
		}
	}
	for _, gone := range []string{"概览", "连接信息"} {
		if tt.HasText(gone) {
			t.Fatalf("inspector still has obsolete tab %q", gone)
		}
	}
}
