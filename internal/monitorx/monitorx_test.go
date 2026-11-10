package monitorx

import (
	"math"
	"testing"
	"time"
)

var firstBlock = []string{
	"cpu  100 0 100 800 0 0 0 0 0 0",
	"MemTotal:       1000 kB",
	"MemAvailable:    400 kB",
	"MemFree:         100 kB",
	"Buffers:          50 kB",
	"Cached:          250 kB",
	"SwapTotal:      2000 kB",
	"SwapFree:       1500 kB",
	"@@DF /dev/sda1 10000 5000 5000 50% /",
	"@@DFS /dev/sda1 10000 5000 5000 50% /",
	"@@DFS /dev/sda2 2000 500 1500 25% /boot",
	"eth0: 1000 0 0 0 0 0 0 0 2000 0 0 0 0 0 0 0",
	"@@LOAD 0.10 0.20 0.30 1/100 123",
	"@@NPROC 4",
	`@@OS "Ubuntu 22.04.4 LTS"`,
	"@@KERNEL Linux 5.15.0-91-generic",
	"@@ARCH x86_64",
	"@@UPTIME 35444321",
	"@@PS java 11.3 1843200;mysqld 3.7 115712;syncthing 0.4 456704;top 0.0 1331;",
}

var secondBlock = []string{
	"cpu  300 0 300 1400 0 0 0 0 0 0",
	"MemTotal:       1000 kB",
	"MemAvailable:    300 kB",
	"MemFree:         100 kB",
	"Buffers:          50 kB",
	"Cached:          150 kB",
	"SwapTotal:      2000 kB",
	"SwapFree:       1400 kB",
	"@@DF /dev/sda1 10000 6000 4000 60% /",
	"eth0: 2000 0 0 0 0 0 0 0 4000 0 0 0 0 0 0 0",
	"@@LOAD 0.40 0.30 0.20 1/100 123",
	"@@NPROC 4",
}

func TestParseLinuxSample(t *testing.T) {
	t0 := time.Unix(1700000000, 0)

	s1, cur, err := parse(firstBlock, nil, t0)
	if err != nil {
		t.Fatal(err)
	}
	if s1.CPU != 0 {
		t.Errorf("first CPU = %v, want 0", s1.CPU)
	}
	if s1.MemTotal != 1000*1024 || s1.MemUsed != 600*1024 {
		t.Errorf("mem = %d/%d", s1.MemUsed, s1.MemTotal)
	}
	if s1.SwapTotal != 2000*1024 || s1.SwapUsed != 500*1024 {
		t.Errorf("swap = %d/%d", s1.SwapUsed, s1.SwapTotal)
	}
	if s1.DiskTotal != 10000*1024 || s1.DiskUsed != 5000*1024 {
		t.Errorf("disk = %d/%d", s1.DiskUsed, s1.DiskTotal)
	}
	if s1.CPUs != 4 {
		t.Errorf("CPUs = %d, want 4", s1.CPUs)
	}
	if s1.OS != "Ubuntu 22.04.4 LTS" || s1.Kernel != "Linux 5.15.0-91-generic" || s1.Arch != "x86_64" {
		t.Errorf("os = %q, kernel = %q, arch = %q", s1.OS, s1.Kernel, s1.Arch)
	}
	if s1.UptimeSec != 35444321 {
		t.Errorf("uptime = %d", s1.UptimeSec)
	}
	if len(s1.Processes) != 4 || s1.Processes[0].Name != "java" ||
		s1.Processes[0].RSS != 1843200*1024 || math.Abs(s1.Processes[0].CPU-11.3) > 0.01 {
		t.Errorf("processes = %+v", s1.Processes)
	}
	if len(s1.Disks) != 2 || s1.Disks[0].Mount != "/" || s1.Disks[0].Total != 10000*1024 ||
		s1.Disks[1].Mount != "/boot" || s1.Disks[1].Used != 500*1024 {
		t.Errorf("disks = %+v", s1.Disks)
	}

	s2, _, err := parse(secondBlock, &cur, t0.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(s2.CPU-40) > 0.01 {
		t.Errorf("CPU = %v, want 40", s2.CPU)
	}
	if s2.MemUsed != 700*1024 {
		t.Errorf("mem used = %d, want %d", s2.MemUsed, 700*1024)
	}
	if s2.SwapUsed != 600*1024 {
		t.Errorf("swap used = %d, want %d", s2.SwapUsed, 600*1024)
	}
	if s2.DiskUsed != 6000*1024 {
		t.Errorf("disk used = %d, want %d", s2.DiskUsed, 6000*1024)
	}
	if math.Abs(s2.RxRate-500) > 0.01 || math.Abs(s2.TxRate-1000) > 0.01 {
		t.Errorf("rates = %v/%v, want 500/1000", s2.RxRate, s2.TxRate)
	}
	if len(s2.Interfaces) != 1 || s2.Interfaces[0].Name != "eth0" ||
		math.Abs(s2.Interfaces[0].RxRate-500) > 0.01 || math.Abs(s2.Interfaces[0].TxRate-1000) > 0.01 {
		t.Errorf("interfaces = %+v, want eth0 at 500/1000", s2.Interfaces)
	}
	if s2.Load1 != 0.4 || s2.Load5 != 0.3 || s2.Load15 != 0.2 {
		t.Errorf("load = %v %v %v", s2.Load1, s2.Load5, s2.Load15)
	}
}

func TestParseRejectsNonLinux(t *testing.T) {
	if _, _, err := parse([]string{"@@NPROC 2"}, nil, time.Now()); err == nil {
		t.Fatal("expected an error without /proc data")
	}
}
