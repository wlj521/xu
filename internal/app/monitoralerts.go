package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"xu/internal/monitorx"
)

const (
	monitorAlertCPU        = "cpu"
	monitorAlertMemory     = "memory"
	monitorAlertDisk       = "disk"
	monitorAlertLatency    = "latency"
	monitorAlertDisconnect = "disconnect"

	monitorPercentMin = 50
	monitorPercentMax = 100
	monitorLatencyMin = 50
	monitorLatencyMax = 5000
)

type monitorSessionAlertState struct {
	initialized bool
	connected   bool
	cpu         bool
	memory      bool
	disk        bool
	latency     bool
}

type monitorAlertNotice struct {
	TermID   int64
	Kind     string
	Server   string
	Message  string
	At       time.Time
	Active   bool
	Critical bool
}

var notifyMonitorDesktop = func(title, body string) {
	if !mygo.NotificationsSupported() {
		return
	}
	_ = mygo.NewNotification(mygo.NotificationOptions{
		Title: title,
		Body:  body,
		Group: "xu-monitor",
	}).Show()
}

func monitorPercentValue(input string) (int, bool) {
	text := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(input), "%"))
	v, err := strconv.Atoi(text)
	return v, err == nil && v >= monitorPercentMin && v <= monitorPercentMax
}

func monitorPercentInput(value int) string {
	if value < monitorPercentMin || value > monitorPercentMax {
		value = 90
	}
	return strconv.Itoa(value)
}

func monitorLatencyValue(input string) (int, bool) {
	text := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(input), "ms"))
	v, err := strconv.Atoi(text)
	return v, err == nil && v >= monitorLatencyMin && v <= monitorLatencyMax
}

func monitorLatencyInput(value int) string {
	if value < monitorLatencyMin || value > monitorLatencyMax {
		value = 500
	}
	return strconv.Itoa(value)
}

func (a *App) recordMonitorSample(ts *termSession, sample monitorx.Sample) {
	if ts == nil {
		return
	}
	ms := a.monitors[ts.ID]
	if ms == nil {
		return
	}
	ms.samples = append(ms.samples, sample)
	if len(ms.samples) > 180 {
		ms.samples = ms.samples[len(ms.samples)-180:]
	}
	a.evaluateMonitorThresholds(ts, sample)
}

func (a *App) evaluateMonitorThresholds(ts *termSession, sample monitorx.Sample) {
	if ts == nil || !a.settings.MonitorAlerts {
		return
	}
	state := a.monitorAlertState[ts.ID]
	if state == nil {
		state = &monitorSessionAlertState{}
		a.monitorAlertState[ts.ID] = state
	}
	server := a.monitorServerLabel(ts)

	cpu := sample.CPU
	a.updateMonitorThreshold(ts, server, &state.cpu, monitorAlertCPU,
		cpu, float64(a.settings.MonitorCPU), 5,
		fmt.Sprintf("%s · CPU %.0f%%（阈值 %d%%）", server, cpu, a.settings.MonitorCPU),
	)

	mem := percent(sample.MemUsed, sample.MemTotal)
	a.updateMonitorThreshold(ts, server, &state.memory, monitorAlertMemory,
		mem, float64(a.settings.MonitorMemory), 5,
		fmt.Sprintf("%s · 内存 %.0f%%（阈值 %d%%）", server, mem, a.settings.MonitorMemory),
	)

	disk, mount, _ := monitorDiskUsage(sample)
	a.updateMonitorThreshold(ts, server, &state.disk, monitorAlertDisk,
		disk, float64(a.settings.MonitorDisk), 5,
		fmt.Sprintf("%s · 磁盘 %s %.0f%%（阈值 %d%%）", server, mount, disk, a.settings.MonitorDisk),
	)

	if sample.LatencyMs > 0 {
		threshold := float64(a.settings.MonitorLatency)
		a.updateMonitorThreshold(ts, server, &state.latency, monitorAlertLatency,
			sample.LatencyMs, threshold, threshold*0.2,
			fmt.Sprintf("%s · 延迟 %.0f ms（阈值 %d ms）", server, sample.LatencyMs, a.settings.MonitorLatency),
		)
	}
}

func (a *App) updateMonitorThreshold(ts *termSession, server string, active *bool, kind string, value, threshold, release float64, message string) {
	if ts == nil || active == nil || threshold <= 0 {
		return
	}
	switch {
	case !*active && value >= threshold:
		*active = true
		a.raiseMonitorAlert(ts, kind, server, message, false)
	case *active && value < threshold-release:
		*active = false
		a.clearMonitorAlert(ts.ID, kind)
	}
}

func monitorDiskUsage(sample monitorx.Sample) (float64, string, uint64) {
	worst := percent(sample.DiskUsed, sample.DiskTotal)
	mount := "/"
	total := sample.DiskTotal
	for _, disk := range sample.Disks {
		if pct := percent(disk.Used, disk.Total); pct > worst {
			worst = pct
			mount = disk.Mount
			total = disk.Total
		}
	}
	return worst, mount, total
}

func (a *App) monitorServerLabel(ts *termSession) string {
	if ts == nil {
		return "未知服务器"
	}
	if ts.profileID != 0 {
		if label := a.transferServerLabel(ts.profileID); label != "" {
			return label
		}
	}
	if endpoint := transferServerFromSSHProfile(ts.profile); endpoint != "" {
		return endpoint
	}
	if ts.Title != "" {
		return ts.Title
	}
	return "服务器"
}

func (a *App) raiseMonitorAlert(ts *termSession, kind, server, message string, critical bool) {
	if a == nil || ts == nil {
		return
	}
	for i := range a.monitorAlerts {
		notice := &a.monitorAlerts[i]
		if notice.Active && notice.TermID == ts.ID && notice.Kind == kind {
			notice.Message = message
			notice.At = time.Now()
			return
		}
	}
	a.monitorAlerts = append([]monitorAlertNotice{{
		TermID:   ts.ID,
		Kind:     kind,
		Server:   server,
		Message:  message,
		At:       time.Now(),
		Active:   true,
		Critical: critical,
	}}, a.monitorAlerts...)
	if len(a.monitorAlerts) > 100 {
		a.monitorAlerts = a.monitorAlerts[:100]
	}
	a.pendingMonitorToasts = append(a.pendingMonitorToasts, message)
	a.status = message

	if a.settings.MonitorDesktop && a.win != nil && !a.win.IsFocused() {
		go notifyMonitorDesktop("Xu · 服务器告警", message)
	}
}

func (a *App) clearMonitorAlert(termID int64, kind string) {
	for i := range a.monitorAlerts {
		notice := &a.monitorAlerts[i]
		if notice.Active && notice.TermID == termID && notice.Kind == kind {
			notice.Active = false
		}
	}
}

func (a *App) activeMonitorAlertCount() int {
	count := 0
	for _, notice := range a.monitorAlerts {
		if notice.Active {
			count++
		}
	}
	return count
}

func (a *App) activeMonitorAlertServerCount() int {
	servers := make(map[int64]bool)
	for _, notice := range a.monitorAlerts {
		if notice.Active {
			servers[notice.TermID] = true
		}
	}
	return len(servers)
}

func (a *App) activeMonitorAlertsForTerm(termID int64) []monitorAlertNotice {
	out := make([]monitorAlertNotice, 0, 2)
	for _, notice := range a.monitorAlerts {
		if notice.Active && notice.TermID == termID {
			out = append(out, notice)
		}
	}
	return out
}

func (a *App) latestActiveMonitorAlertForTerm(termID int64) (monitorAlertNotice, bool) {
	for _, notice := range a.monitorAlerts {
		if !notice.Active || notice.TermID != termID {
			continue
		}
		if notice.Critical {
			return notice, true
		}
	}
	for _, notice := range a.monitorAlerts {
		if notice.Active && notice.TermID == termID {
			return notice, true
		}
	}
	return monitorAlertNotice{}, false
}

func (a *App) syncMonitorConnectionAlerts() {
	if a == nil {
		return
	}
	present := make(map[int64]bool, len(a.terms))
	for _, ts := range a.terms {
		if ts == nil || ts.local {
			continue
		}
		present[ts.ID] = true
		state := a.monitorAlertState[ts.ID]
		if state == nil {
			state = &monitorSessionAlertState{}
			a.monitorAlertState[ts.ID] = state
		}
		connected := !ts.exited
		if !state.initialized {
			state.initialized = true
			state.connected = connected
			if !connected {
				state.cpu = false
				state.memory = false
				state.disk = false
				state.latency = false
				a.clearMonitorThresholdAlertsForTerm(ts.ID)
			}
			continue
		}
		switch {
		case state.connected && !connected:
			state.cpu = false
			state.memory = false
			state.disk = false
			state.latency = false
			a.clearMonitorThresholdAlertsForTerm(ts.ID)
			if a.settings.MonitorDisconnect {
				a.raiseMonitorAlert(ts, monitorAlertDisconnect, a.monitorServerLabel(ts),
					a.monitorServerLabel(ts)+" · SSH 连接已断开", true)
			}
		case !state.connected && connected:
			a.clearMonitorAlert(ts.ID, monitorAlertDisconnect)
		}
		state.connected = connected
	}

	for id := range a.monitorAlertState {
		if present[id] {
			continue
		}
		delete(a.monitorAlertState, id)
		a.clearMonitorAlertsForTerm(id)
	}
	if !a.settings.MonitorDisconnect {
		a.clearMonitorAlertKind(monitorAlertDisconnect)
	}
}

func (a *App) forgetMonitorAlerts(termID int64) {
	delete(a.monitorAlertState, termID)
	a.clearMonitorAlertsForTerm(termID)
}

func (a *App) clearMonitorAlertsForTerm(termID int64) {
	for i := range a.monitorAlerts {
		if a.monitorAlerts[i].TermID == termID {
			a.monitorAlerts[i].Active = false
		}
	}
}

func (a *App) clearMonitorThresholdAlertsForTerm(termID int64) {
	for i := range a.monitorAlerts {
		notice := &a.monitorAlerts[i]
		if notice.TermID != termID {
			continue
		}
		switch notice.Kind {
		case monitorAlertCPU, monitorAlertMemory, monitorAlertDisk, monitorAlertLatency:
			notice.Active = false
		}
	}
}

func (a *App) clearMonitorAlertKind(kind string) {
	for i := range a.monitorAlerts {
		if a.monitorAlerts[i].Kind == kind {
			a.monitorAlerts[i].Active = false
		}
	}
}

func (a *App) resetMonitorThresholdAlerts() {
	for _, state := range a.monitorAlertState {
		state.cpu = false
		state.memory = false
		state.disk = false
		state.latency = false
	}
	for i := range a.monitorAlerts {
		switch a.monitorAlerts[i].Kind {
		case monitorAlertCPU, monitorAlertMemory, monitorAlertDisk, monitorAlertLatency:
			a.monitorAlerts[i].Active = false
		}
	}
}

func (a *App) flushMonitorAlertToasts(c *ui.Context) {
	if c == nil {
		return
	}
	n := min(3, len(a.pendingMonitorToasts))
	for _, message := range a.pendingMonitorToasts[:n] {
		c.Toast(message)
	}
	a.pendingMonitorToasts = a.pendingMonitorToasts[n:]
}
