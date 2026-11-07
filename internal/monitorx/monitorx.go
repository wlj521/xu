package monitorx

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

type Process struct {
	Name string
	RSS  uint64
}

type Sample struct {
	At        time.Time
	CPU       float64
	MemUsed   uint64
	MemTotal  uint64
	SwapUsed  uint64
	SwapTotal uint64
	DiskUsed  uint64
	DiskTotal uint64
	RxRate    float64
	TxRate    float64
	Load1     float64
	Load5     float64
	Load15    float64
	CPUs      int
	OS        string
	Kernel    string
	Arch      string
	UptimeSec uint64
	Processes []Process
}

type Collector struct {
	sess   *ssh.Session
	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
}

func Start(parent context.Context, client *ssh.Client, interval time.Duration, onSample func(Sample), onErr func(error)) (*Collector, error) {
	if interval < time.Second {
		interval = 2 * time.Second
	}

	sess, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	out, err := sess.StdoutPipe()
	if err != nil {
		sess.Close()
		return nil, err
	}
	sess.Stderr = io.Discard

	seconds := int(interval.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	script := fmt.Sprintf(`LC_ALL=C
while :; do
  echo "@@XU_BEGIN"
  head -n 1 /proc/stat 2>/dev/null
  grep -E '^(MemTotal|MemAvailable|MemFree|Buffers|Cached|SwapTotal|SwapFree):' /proc/meminfo 2>/dev/null
  echo "@@DF $(df -P -k / 2>/dev/null | tail -n 1)"
  cat /proc/net/dev 2>/dev/null
  echo "@@LOAD $(cat /proc/loadavg 2>/dev/null)"
  echo "@@NPROC $(nproc 2>/dev/null || getconf _NPROCESSORS_ONLN 2>/dev/null || echo 1)"
  echo "@@OS $(. /etc/os-release 2>/dev/null; printf '%%s' \"$PRETTY_NAME\")"
  echo "@@KERNEL $(uname -sr 2>/dev/null)"
  echo "@@ARCH $(uname -m 2>/dev/null)"
  echo "@@UPTIME $(cut -d. -f1 /proc/uptime 2>/dev/null)"
  echo "@@PS $(ps -eo comm=,rss= --sort=-rss 2>/dev/null | head -4 | tr '\n' ';')"
  echo "@@XU_END"
  sleep %d
done`, seconds)

	if err := sess.Start("sh -c " + shellQuote(script)); err != nil {
		sess.Close()
		return nil, err
	}

	ctx, cancel := context.WithCancel(parent)
	c := &Collector{sess: sess, cancel: cancel, done: make(chan struct{})}
	go c.read(ctx, out, onSample, onErr)
	return c, nil
}

func (c *Collector) Stop() {
	c.once.Do(func() {
		c.cancel()
		_ = c.sess.Signal(ssh.SIGTERM)
		c.sess.Close()
	})
	<-c.done
}

func (c *Collector) read(ctx context.Context, r io.Reader, onSample func(Sample), onErr func(error)) {
	defer close(c.done)

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)

	var block []string
	var prev *raw
	inBlock := false

	for scanner.Scan() {
		line := scanner.Text()
		switch line {
		case "@@XU_BEGIN":
			block = block[:0]
			inBlock = true
		case "@@XU_END":
			if inBlock {
				sample, cur, err := parse(block, prev, time.Now())
				if err != nil {
					if onErr != nil {
						onErr(err)
					}
				} else {
					prev = &cur
					if onSample != nil {
						onSample(sample)
					}
				}
			}
			inBlock = false
		default:
			if inBlock {
				block = append(block, line)
			}
		}
	}

	if ctx.Err() == nil {
		err := scanner.Err()
		if err == nil {
			err = io.EOF
		}
		if onErr != nil {
			onErr(err)
		}
	}
}

type raw struct {
	at       time.Time
	cpuTotal uint64
	cpuIdle  uint64
	rx       uint64
	tx       uint64
}

func parse(lines []string, prev *raw, at time.Time) (Sample, raw, error) {
	var s Sample
	s.At = at
	cur := raw{at: at}

	var memTotal, memAvail, memFree, buffers, cached uint64
	var swapTotal, swapFree uint64
	haveCPU, haveMem, haveDisk := false, false, false

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "cpu "):
			fields := strings.Fields(line)
			if len(fields) < 5 {
				return s, cur, fmt.Errorf("无法解析 /proc/stat: %q", line)
			}
			var total, idle uint64
			for i, f := range fields[1:] {
				v, err := strconv.ParseUint(f, 10, 64)
				if err != nil {
					return s, cur, fmt.Errorf("无法解析 CPU 时间: %q", line)
				}
				total += v
				if i == 3 || i == 4 {
					idle += v
				}
			}
			cur.cpuTotal, cur.cpuIdle = total, idle
			haveCPU = true

		case strings.HasPrefix(line, "MemTotal:"):
			memTotal = parseKB(line)
			haveMem = true
		case strings.HasPrefix(line, "MemAvailable:"):
			memAvail = parseKB(line)
		case strings.HasPrefix(line, "MemFree:"):
			memFree = parseKB(line)
		case strings.HasPrefix(line, "Buffers:"):
			buffers = parseKB(line)
		case strings.HasPrefix(line, "Cached:"):
			cached = parseKB(line)
		case strings.HasPrefix(line, "SwapTotal:"):
			swapTotal = parseKB(line)
		case strings.HasPrefix(line, "SwapFree:"):
			swapFree = parseKB(line)

		case strings.HasPrefix(line, "@@DF "):
			fields := strings.Fields(strings.TrimPrefix(line, "@@DF "))
			if len(fields) >= 3 {
				total, err1 := strconv.ParseUint(fields[1], 10, 64)
				used, err2 := strconv.ParseUint(fields[2], 10, 64)
				if err1 == nil && err2 == nil {
					s.DiskTotal = total * 1024
					s.DiskUsed = used * 1024
					haveDisk = true
				}
			}

		case strings.HasPrefix(line, "@@LOAD "):
			fields := strings.Fields(strings.TrimPrefix(line, "@@LOAD "))
			if len(fields) >= 3 {
				s.Load1, _ = strconv.ParseFloat(fields[0], 64)
				s.Load5, _ = strconv.ParseFloat(fields[1], 64)
				s.Load15, _ = strconv.ParseFloat(fields[2], 64)
			}

		case strings.HasPrefix(line, "@@NPROC "):
			v, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "@@NPROC ")))
			if v > 0 {
				s.CPUs = v
			}

		case strings.HasPrefix(line, "@@OS "):
			s.OS = strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "@@OS ")), `"`)

		case strings.HasPrefix(line, "@@KERNEL "):
			s.Kernel = strings.TrimSpace(strings.TrimPrefix(line, "@@KERNEL "))

		case strings.HasPrefix(line, "@@ARCH "):
			s.Arch = strings.TrimSpace(strings.TrimPrefix(line, "@@ARCH "))

		case strings.HasPrefix(line, "@@UPTIME "):
			if v, err := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(line, "@@UPTIME ")), 10, 64); err == nil {
				s.UptimeSec = v
			}

		case strings.HasPrefix(line, "@@PS "):
			rest := strings.TrimSpace(strings.TrimPrefix(line, "@@PS "))
			for _, entry := range strings.Split(rest, ";") {
				fields := strings.Fields(entry)
				if len(fields) < 2 {
					continue
				}
				rss, err := strconv.ParseUint(fields[len(fields)-1], 10, 64)
				if err != nil {
					continue
				}
				s.Processes = append(s.Processes, Process{
					Name: strings.Join(fields[:len(fields)-1], " "),
					RSS:  rss * 1024,
				})
			}

		case strings.Contains(line, ":"):
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}
			iface := strings.TrimSpace(parts[0])
			if iface == "" || iface == "lo" {
				continue
			}
			fields := strings.Fields(parts[1])
			if len(fields) >= 9 {
				rx, err1 := strconv.ParseUint(fields[0], 10, 64)
				tx, err2 := strconv.ParseUint(fields[8], 10, 64)
				if err1 == nil && err2 == nil {
					cur.rx += rx
					cur.tx += tx
				}
			}
		}
	}

	if !haveCPU || !haveMem || memTotal == 0 {
		return s, cur, errors.New("仅支持 Linux：缺少 /proc/stat 或 /proc/meminfo")
	}
	_ = haveDisk

	s.MemTotal = memTotal
	if memAvail == 0 {
		memAvail = memFree + buffers + cached
	}
	if memAvail > memTotal {
		memAvail = memTotal
	}
	s.MemUsed = memTotal - memAvail

	s.SwapTotal = swapTotal
	if swapFree <= swapTotal {
		s.SwapUsed = swapTotal - swapFree
	}

	if s.CPUs == 0 {
		s.CPUs = 1
	}
	if s.OS == "" {
		s.OS = "Linux"
	}

	if prev != nil {
		if cur.cpuTotal > prev.cpuTotal {
			dt := cur.cpuTotal - prev.cpuTotal
			di := cur.cpuIdle - prev.cpuIdle
			s.CPU = clamp(100*(1-float64(di)/float64(dt)), 0, 100)
		}
		secs := at.Sub(prev.at).Seconds()
		if secs > 0 {
			if cur.rx >= prev.rx {
				s.RxRate = float64(cur.rx-prev.rx) / secs
			}
			if cur.tx >= prev.tx {
				s.TxRate = float64(cur.tx-prev.tx) / secs
			}
		}
	}

	return s, cur, nil
}

func parseKB(line string) uint64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	v, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0
	}
	return v * 1024
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
