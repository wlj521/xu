package app

import (
	"context"
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"xu/internal/store"
)

var logSGR = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripLogSGR(s string) string { return logSGR.ReplaceAllString(s, "") }

func mustLogConfig(t *testing.T, enabled, anchor bool, rules []logRule) *logConfig {
	t.Helper()
	cfg, errs := compileLogConfig(enabled, anchor, rules, nil)
	if len(errs) != 0 {
		t.Fatalf("compile errors: %v", errs)
	}
	return cfg
}

func highlightLog(cfg *logConfig, input string) string {
	st := &logHighlightState{clean: true}
	return string(st.process(cfg, []byte(input), nil))
}

func TestLogHighlightGeneralLine(t *testing.T) {
	cfg := mustLogConfig(t, true, true, defaultLogRules())
	input := "2026-11-11 13:21:05.402 INFO  [jt808-business-6-3] com.peak.jt808.handler.RawForwardMessageHandler - started\n"
	out := highlightLog(cfg, input)
	if got := stripLogSGR(out); got != input {
		t.Fatalf("text changed:\n got %q\nwant %q", got, input)
	}
	for _, want := range []string{"\x1b[38;5;8m", "\x1b[38;5;2mINFO", "\x1b[38;5;6m", "\x1b[38;5;5m"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}

func TestLogHighlightCustomFormat(t *testing.T) {
	rules := defaultLogRules()
	input := "[MYAPP] 11-11 14:52:04 INFO  com.example.Foo - started\n"

	builtin := mustLogConfig(t, true, true, rules)
	if out := highlightLog(builtin, input); strings.Contains(out, "\x1b[") {
		t.Fatalf("the line matched a built-in format: %q", out)
	}

	anchors, errs := compileLogFormats([]logFormat{
		{ID: 1, Pattern: `^\[MYAPP\]`, Enabled: true},
	})
	if len(errs) != 0 {
		t.Fatalf("format errors: %v", errs)
	}
	cfg, _ := compileLogConfig(true, true, rules, anchors)
	out := highlightLog(cfg, input)
	if !strings.Contains(out, "\x1b[38;5;2mINFO") {
		t.Fatalf("the custom format did not admit the line: %q", out)
	}
	if got := stripLogSGR(out); got != input {
		t.Fatalf("text changed: %q", got)
	}
}

func TestLogFormatCompileError(t *testing.T) {
	anchors, errs := compileLogFormats([]logFormat{
		{ID: 3, Pattern: "(", Enabled: true},
		{ID: 4, Pattern: "", Enabled: true},
	})
	if errs[3] == "" || errs[4] == "" {
		t.Fatalf("errors = %v, want both formats reported", errs)
	}
	if len(anchors) != 0 {
		t.Fatalf("anchors = %d, want none", len(anchors))
	}
}

func TestLogHighlightTraceIDPrefix(t *testing.T) {
	cfg := mustLogConfig(t, true, true, defaultLogRules())
	input := "[530d461d46c544978af8fa28fa344bcc] 2026-11-11 14:52:04.231 [http-nio-8085-exec-19] INFO  com.peak.ddt.config.aspect.LogAspect - 返回结果: {\"code\":200}\r\n"
	out := highlightLog(cfg, input)
	if got := stripLogSGR(out); got != input {
		t.Fatalf("text changed: %q", got)
	}
	for _, want := range []string{
		"\x1b[38;5;8m[530d461d46c544978af8fa28fa344bcc]",
		"\x1b[38;5;2mINFO",
		"\x1b[38;5;6m[http-nio-8085-exec-19]",
		"\x1b[38;5;5mcom.peak.ddt.config.aspect.LogAspect",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}

func TestLogHighlightErrorBold(t *testing.T) {
	cfg := mustLogConfig(t, true, true, defaultLogRules())
	out := highlightLog(cfg, "2026-11-11 13:21:07.105 ERROR connection reset\n")
	if !strings.Contains(out, "\x1b[38;5;1m\x1b[1mERROR") {
		t.Fatalf("ERROR is not red and bold: %q", out)
	}
}

func TestLogHighlightAnchorOnly(t *testing.T) {
	input := "note INFO here\n"
	anchored := mustLogConfig(t, true, true, defaultLogRules())
	if out := highlightLog(anchored, input); strings.Contains(out, "\x1b[") {
		t.Fatalf("non-log line was colored: %q", out)
	}
	open := mustLogConfig(t, true, false, defaultLogRules())
	if out := highlightLog(open, input); !strings.Contains(out, "\x1b[38;5;2mINFO") {
		t.Fatalf("INFO was not colored with anchoring off: %q", out)
	}
}

func TestLogHighlightSkipsESC(t *testing.T) {
	cfg := mustLogConfig(t, true, true, defaultLogRules())
	input := "\x1b[31mred\x1b[0m INFO\n2026-11-11 13:21:05.402 INFO ok\n"
	out := highlightLog(cfg, input)
	parts := strings.SplitN(out, "\n", 2)
	if parts[0] != "\x1b[31mred\x1b[0m INFO" {
		t.Fatalf("the colored line changed: %q", parts[0])
	}
	if !strings.Contains(parts[1], "\x1b[38;5;2mINFO") {
		t.Fatalf("the next line was not colored: %q", parts[1])
	}
}

func TestLogHighlightCRLF(t *testing.T) {
	cfg := mustLogConfig(t, true, true, defaultLogRules())
	input := "2026-11-11 13:21:05.402 INFO ok\r\n"
	out := highlightLog(cfg, input)
	if got := stripLogSGR(out); got != input {
		t.Fatalf("text changed: %q", got)
	}
	if !strings.Contains(out, "\x1b[38;5;2mINFO") {
		t.Fatalf("CRLF line was not colored: %q", out)
	}
}

func TestLogHighlightProgress(t *testing.T) {
	cfg := mustLogConfig(t, true, true, defaultLogRules())
	input := "downloading 10%\rdownloading 20%\n"
	if out := highlightLog(cfg, input); out != input {
		t.Fatalf("progress output changed: %q", out)
	}
}

func TestLogHighlightAltScreen(t *testing.T) {
	cfg := mustLogConfig(t, true, true, defaultLogRules())
	line := "2026-11-11 13:21:05.402 INFO ok\n"
	st := &logHighlightState{clean: true}
	_ = st.process(cfg, []byte("\x1b[?1049h"), nil)
	if out := string(st.process(cfg, []byte(line), nil)); out != line {
		t.Fatalf("alternate screen output changed: %q", out)
	}
	_ = st.process(cfg, []byte("\x1b[?1049l"), nil)
	if out := string(st.process(cfg, []byte(line), nil)); !strings.Contains(out, "\x1b[38;5;2mINFO") {
		t.Fatalf("the line after leaving the alternate screen was not colored: %q", out)
	}
}

func TestLogHighlightChunkSplit(t *testing.T) {
	cfg := mustLogConfig(t, true, true, defaultLogRules())
	st := &logHighlightState{clean: true}
	out := string(st.process(cfg, []byte("2026-11-11 13:21:05.402 IN"), nil))
	out += string(st.process(cfg, []byte("FO ok\n"), nil))
	if got := stripLogSGR(out); got != "2026-11-11 13:21:05.402 INFO ok\n" {
		t.Fatalf("text changed across chunks: %q", got)
	}
	if !strings.Contains(out, "\x1b[38;5;8m") {
		t.Fatalf("the first chunk was not colored: %q", out)
	}
}

func TestLogHighlightKeyword(t *testing.T) {
	cfg := mustLogConfig(t, true, false, []logRule{
		{ID: 1, Pattern: "boom", Keyword: true, Color: 1, Enabled: true},
	})
	out := highlightLog(cfg, "before boom after\n")
	if !strings.Contains(out, "\x1b[38;5;1mboom") {
		t.Fatalf("keyword was not colored: %q", out)
	}
	if got := stripLogSGR(out); got != "before boom after\n" {
		t.Fatalf("text changed: %q", got)
	}
}

func TestLogHighlightWholeLine(t *testing.T) {
	cfg := mustLogConfig(t, true, false, []logRule{
		{ID: 1, Pattern: `INFO`, Color: 2, Enabled: true},
		{ID: 2, Pattern: `INFO`, Color: 5, Whole: true, Enabled: true},
	})
	out := highlightLog(cfg, "x INFO y\n")
	if !strings.Contains(out, "\x1b[38;5;2mINFO") {
		t.Fatalf("the earlier INFO rule lost its span: %q", out)
	}
	if strings.Count(out, "\x1b[38;5;5m") < 2 {
		t.Fatalf("the whole-line rule did not fill both gaps: %q", out)
	}
	if got := stripLogSGR(out); got != "x INFO y\n" {
		t.Fatalf("text changed: %q", got)
	}
	if strings.Contains(out, "\x1b[38;5;2m\x1b[38;5;5m") {
		t.Fatalf("nested colors: %q", out)
	}
}

func TestLogHighlightWholeLineNeedsMatch(t *testing.T) {
	cfg := mustLogConfig(t, true, false, []logRule{
		{ID: 1, Pattern: `ERROR`, Color: 5, Whole: true, Enabled: true},
	})
	if out := highlightLog(cfg, "INFO ok\n"); strings.Contains(out, "\x1b[") {
		t.Fatalf("a whole-line rule colored a line it does not match: %q", out)
	}
}

func TestLogHighlightInvalidRegex(t *testing.T) {
	cfg, errs := compileLogConfig(true, false, []logRule{
		{ID: 1, Pattern: "(", Color: 1, Enabled: true},
		{ID: 2, Pattern: "ok", Keyword: true, Color: 2, Enabled: true},
	}, nil)
	if errs[1] == "" {
		t.Fatal("invalid regex was not reported")
	}
	if len(cfg.rules) != 1 {
		t.Fatalf("compiled rules = %d, want the one valid rule", len(cfg.rules))
	}
}

func TestLogHighlightDisabled(t *testing.T) {
	cfg := mustLogConfig(t, false, true, defaultLogRules())
	input := "2026-11-11 13:21:05.402 INFO ok\n"
	if out := highlightLog(cfg, input); out != input {
		t.Fatalf("disabled highlighting changed the line: %q", out)
	}
}

func TestLogHighlightJSONPreset(t *testing.T) {
	cfg := mustLogConfig(t, true, true, jsonLogRules())
	input := `{"timestamp":"2026-11-11T13:21:05Z","level":"ERROR","message":"boom"}` + "\n"
	out := highlightLog(cfg, input)
	if !strings.Contains(out, "\x1b[38;5;1m\x1b[1m") {
		t.Fatalf("JSON ERROR was not colored: %q", out)
	}
	if got := stripLogSGR(out); got != input {
		t.Fatalf("JSON text changed: %q", got)
	}
}

type logChunkConn struct {
	chunks [][]byte
	chunk  int
	pos    int
	closed bool
}

func (c *logChunkConn) Read(p []byte) (int, error) {
	if c.chunk >= len(c.chunks) {
		return 0, io.EOF
	}
	n := copy(p, c.chunks[c.chunk][c.pos:])
	c.pos += n
	if c.pos >= len(c.chunks[c.chunk]) {
		c.chunk++
		c.pos = 0
	}
	return n, nil
}

func (c *logChunkConn) Write(p []byte) (int, error) { return len(p), nil }
func (c *logChunkConn) Close() error {
	c.closed = true
	return nil
}
func (c *logChunkConn) Resize(cols, rows int) error { return nil }

func TestHighlightConnReadsSmallBuffers(t *testing.T) {
	input := "2026-11-11 13:21:05.402 INFO ok\n"
	src := &logChunkConn{chunks: [][]byte{
		[]byte("2026-11-11 13:21:05.402 INFO ok\n"),
	}}
	hl := &logHighlighter{}
	hl.store(mustLogConfig(t, true, true, defaultLogRules()))
	conn := newHighlightConn(src, hl)
	defer conn.Close()

	var got []byte
	buf := make([]byte, 5)
	for {
		n, err := conn.Read(buf)
		got = append(got, buf[:n]...)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read: %v", err)
		}
	}
	if got := stripLogSGR(string(got)); got != input {
		t.Fatalf("text changed: %q", got)
	}
	if !strings.Contains(string(got), "\x1b[38;5;2mINFO") {
		t.Fatalf("output was not colored: %q", got)
	}
}

func TestApplyLogHighlightCompiles(t *testing.T) {
	a := New()
	cfg := a.logHL.load()
	if cfg == nil || !cfg.enabled || len(cfg.rules) == 0 {
		t.Fatalf("defaults were not compiled: %+v", cfg)
	}
	a.settings.LogHighlight = false
	a.applyLogHighlight()
	if a.logHL.load().enabled {
		t.Fatal("the disabled switch was not compiled")
	}
	a.settings.LogHighlight = true
	a.settings.LogRules = []logRule{{ID: 99, Pattern: "(", Enabled: true}}
	a.applyLogHighlight()
	if a.logRuleErrs[99] == "" {
		t.Fatal("the invalid rule was not reported")
	}
}

func TestSettingsLogHighlightGroup(t *testing.T) {
	a := New()
	a.openSettings()
	a.settingsSection = 1
	a.logRulesOpen = true

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	for _, want := range []string{"日志高亮", "启用日志高亮", "仅处理日志行", "预设", "规则与格式"} {
		if !tt.HasText(want) {
			t.Fatalf("terminal settings is missing %q; texts = %q", want, tt.Texts())
		}
	}
	for _, want := range []string{"日志高亮", "规则", "日志格式", "整行", "加粗", "添加规则", "恢复默认"} {
		if !tt.HasText(want) {
			t.Fatalf("rules dialog is missing %q; texts = %q", want, tt.Texts())
		}
	}

	before := len(a.settings.LogRules)
	if err := tt.Click("添加规则"); err != nil {
		t.Fatalf("click 添加规则: %v", err)
	}
	tt.Frame()
	if len(a.settings.LogRules) != before+1 {
		t.Fatalf("rules = %d, want %d", len(a.settings.LogRules), before+1)
	}

	if err := tt.Click("日志格式"); err != nil {
		t.Fatalf("click 日志格式: %v", err)
	}
	tt.Frame()
	if a.logRulesTab != 1 {
		t.Fatalf("tab = %d, want the format page", a.logRulesTab)
	}
	for _, want := range []string{"添加格式", "清空", "还没有自定义格式", "内置格式", "示例", "测试一行", "使用"} {
		if !tt.HasText(want) {
			t.Fatalf("format page is missing %q; texts = %q", want, tt.Texts())
		}
	}
	formats := len(a.settings.LogFormats)
	if err := tt.Click("添加格式"); err != nil {
		t.Fatalf("click 添加格式: %v", err)
	}
	tt.Frame()
	if len(a.settings.LogFormats) != formats+1 {
		t.Fatalf("formats = %d, want %d", len(a.settings.LogFormats), formats+1)
	}
}

type logEOFConn struct{ data []byte }

func (c *logEOFConn) Read(p []byte) (int, error) {
	if len(c.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, c.data)
	c.data = c.data[n:]
	return n, io.EOF
}
func (c *logEOFConn) Write(p []byte) (int, error) { return len(p), nil }
func (c *logEOFConn) Close() error                { return nil }

func TestHighlightConnDeliversDataWithEOF(t *testing.T) {
	input := "2026-11-11 13:21:05.402 INFO ok\n"
	hl := &logHighlighter{}
	hl.store(mustLogConfig(t, true, true, defaultLogRules()))
	conn := newHighlightConn(&logEOFConn{data: []byte(input)}, hl)

	var got []byte
	buf := make([]byte, 4)
	for {
		n, err := conn.Read(buf)
		got = append(got, buf[:n]...)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read: %v", err)
		}
	}
	if stripLogSGR(string(got)) != input {
		t.Fatalf("text changed: %q", got)
	}
	if !strings.Contains(string(got), "\x1b[38;5;2mINFO") {
		t.Fatalf("output was not colored: %q", got)
	}
}

func TestLogHighlightSettingsRoundTrip(t *testing.T) {
	a := New()
	a.settings.LogHighlight = false
	a.settings.LogAnchorOnly = false
	a.settings.LogRules = []logRule{
		{ID: 7, Name: "自定义", Pattern: "hello", Keyword: true, Color: 3, Bold: true, Enabled: true},
	}
	a.settings.LogFormats = []logFormat{
		{ID: 8, Name: "MyApp", Pattern: `^\[MYAPP\]`, Enabled: true},
	}
	raw := a.logHighlightValue()

	b := New()
	b.loadLogHighlight(raw)
	if b.settings.LogHighlight || b.settings.LogAnchorOnly {
		t.Fatalf("flags were not loaded: %+v", b.settings)
	}
	if len(b.settings.LogRules) != 1 || b.settings.LogRules[0].Pattern != "hello" ||
		b.settings.LogRules[0].Color != 3 || !b.settings.LogRules[0].Bold {
		t.Fatalf("rules were not loaded: %+v", b.settings.LogRules)
	}
	if len(b.settings.LogFormats) != 1 || b.settings.LogFormats[0].Pattern != `^\[MYAPP\]` {
		t.Fatalf("formats were not loaded: %+v", b.settings.LogFormats)
	}
}

func TestLogHighlightPersists(t *testing.T) {
	mygo.App.SetPath(mygo.PathUserData, t.TempDir())
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })

	st, err := store.Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	a := New()
	a.SetStore(st)
	a.settings.LogHighlight = false
	a.settings.LogRules = []logRule{
		{ID: 11, Name: "自定义", Pattern: "boom", Color: 1, Enabled: true},
	}
	a.saveSettings()
	if a.settingsErr != "" {
		t.Fatalf("save settings: %s", a.settingsErr)
	}

	b := New()
	b.SetStore(st)
	if b.settings.LogHighlight {
		t.Fatal("the switch was not loaded")
	}
	if len(b.settings.LogRules) != 1 || b.settings.LogRules[0].Pattern != "boom" {
		t.Fatalf("rules were not loaded: %+v", b.settings.LogRules)
	}
}
