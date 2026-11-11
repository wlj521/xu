package app

import (
	"bytes"
	"encoding/json"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// logRule is one log highlighting rule. Pattern is a Go regular expression,
// or a literal keyword when Keyword is set. Color is an index into the
// terminal's 16-color palette, so rules follow the selected color scheme.
type logRule struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Pattern string `json:"pattern"`
	Keyword bool   `json:"keyword"`
	Color   int    `json:"color"`
	Bold    bool   `json:"bold"`
	Whole   bool   `json:"whole"`
	Enabled bool   `json:"enabled"`
}

// logFormat is a user-defined way to recognize a log line: a regular
// expression matched from the start of the line. The built-in shapes stay
// active, so a format only adds to them.
type logFormat struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Pattern string `json:"pattern"`
	Enabled bool   `json:"enabled"`
}

const (
	maxLogSpans     = 40
	maxLogLineBytes = 64 << 10
)

// defaultLogRules is the general log pack: the shapes most Java, Go and
// Python services write to a file.
func defaultLogRules() []logRule {
	return []logRule{
		{ID: 11, Name: "Trace ID", Pattern: `^\[[0-9a-fA-F][0-9a-fA-F-]{15,63}\]`, Color: 8, Enabled: true},
		{ID: 1, Name: "时间戳", Pattern: `\b\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}:\d{2}(?:[.,]\d{1,6})?\b`, Color: 8, Enabled: true},
		{ID: 2, Name: "TRACE", Pattern: `\bTRACE\b`, Color: 8, Enabled: true},
		{ID: 3, Name: "DEBUG", Pattern: `\bDEBUG\b`, Color: 4, Enabled: true},
		{ID: 4, Name: "INFO", Pattern: `\bINFO\b`, Color: 2, Enabled: true},
		{ID: 5, Name: "WARN", Pattern: `\bWARN(?:ING)?\b`, Color: 3, Enabled: true},
		{ID: 6, Name: "ERROR", Pattern: `\b(?:ERROR|FATAL)\b`, Color: 1, Bold: true, Enabled: true},
		{ID: 7, Name: "线程", Pattern: `\[[^\]\r\n]{1,80}\]`, Color: 6, Enabled: true},
		{ID: 8, Name: "logger", Pattern: `\b(?:[a-zA-Z_$][\w$]*\.){2,}[A-Z][\w$]*\b`, Color: 5, Enabled: true},
		{ID: 9, Name: "IP", Pattern: `\b(?:\d{1,3}\.){3}\d{1,3}\b`, Color: 14, Enabled: true},
		{ID: 10, Name: "HTTP 方法", Pattern: `\b(?:GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)\b`, Color: 4, Bold: true, Enabled: true},
	}
}

func nginxLogRules() []logRule {
	return []logRule{
		{ID: 1, Name: "客户端 IP", Pattern: `^\d{1,3}(?:\.\d{1,3}){3}`, Color: 14, Enabled: true},
		{ID: 2, Name: "时间", Pattern: `\[[^\]]+\]`, Color: 8, Enabled: true},
		{ID: 3, Name: "请求", Pattern: `"(?:GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS) [^"]*"`, Color: 4, Bold: true, Enabled: true},
		{ID: 4, Name: "2xx", Pattern: `\s2\d{2}\s`, Color: 2, Enabled: true},
		{ID: 5, Name: "3xx", Pattern: `\s3\d{2}\s`, Color: 6, Enabled: true},
		{ID: 6, Name: "4xx", Pattern: `\s4\d{2}\s`, Color: 3, Enabled: true},
		{ID: 7, Name: "5xx", Pattern: `\s5\d{2}\s`, Color: 1, Bold: true, Enabled: true},
	}
}

func jsonLogRules() []logRule {
	return []logRule{
		{ID: 1, Name: "时间戳", Pattern: `"@?timestamp"\s*:\s*"[^"]+"`, Color: 8, Enabled: true},
		{ID: 2, Name: "TRACE", Pattern: `"level"\s*:\s*"TRACE"`, Color: 8, Enabled: true},
		{ID: 3, Name: "DEBUG", Pattern: `"level"\s*:\s*"DEBUG"`, Color: 4, Enabled: true},
		{ID: 4, Name: "INFO", Pattern: `"level"\s*:\s*"INFO"`, Color: 2, Enabled: true},
		{ID: 5, Name: "WARN", Pattern: `"level"\s*:\s*"WARN(?:ING)?"`, Color: 3, Enabled: true},
		{ID: 6, Name: "ERROR", Pattern: `"level"\s*:\s*"(?:ERROR|FATAL)"`, Color: 1, Bold: true, Enabled: true},
		{ID: 7, Name: "logger", Pattern: `"logger"\s*:\s*"[^"]+"`, Color: 5, Enabled: true},
		{ID: 8, Name: "message", Pattern: `"message"\s*:\s*"[^"]*"`, Color: 7, Enabled: true},
	}
}

func logPresetNames() []string {
	return []string{"通用日志", "Nginx", "JSON 日志", "自定义"}
}

func logRulesForPreset(name string) []logRule {
	switch name {
	case "Nginx":
		return nginxLogRules()
	case "JSON 日志":
		return jsonLogRules()
	case "自定义":
		return nil
	default:
		return defaultLogRules()
	}
}

func nextLogRuleID() int64 { return time.Now().UnixNano() }

// logAnchor tells a log line from ordinary command output. It covers the
// common timestamp, level-first, Nginx and JSON shapes.
var logAnchor = regexp.MustCompile(`^.{0,160}?(?:\d{4}[-/]\d{2}[-/]\d{2}[ T]\d{2}:\d{2}:\d{2}|\[?\d{2}:\d{2}:\d{2}[.,]\d+|\[(?:TRACE|DEBUG|INFO|WARN|WARNING|ERROR|FATAL)\]|\d{1,3}(?:\.\d{1,3}){3}\s+-\s+-|\{\s*"(?:@?timestamp|level|severity|time|logger)")`)

type compiledLogRule struct {
	id      int64
	re      *regexp.Regexp
	kw      []byte
	keyword bool
	color   int
	bold    bool
	whole   bool
}

type logConfig struct {
	enabled    bool
	anchorOnly bool
	anchors    []*regexp.Regexp
	rules      []compiledLogRule
}

func (c *logConfig) isLogLine(line []byte) bool {
	if logAnchor.Match(line) {
		return true
	}
	for _, anchor := range c.anchors {
		if anchor != nil && anchor.Match(line) {
			return true
		}
	}
	return false
}

// compileLogFormats compiles the user's line formats, anchoring each from
// the start of the line.
func compileLogFormats(formats []logFormat) ([]*regexp.Regexp, map[int64]string) {
	var anchors []*regexp.Regexp
	errs := map[int64]string{}
	for _, format := range formats {
		if !format.Enabled {
			continue
		}
		pattern := strings.TrimSpace(format.Pattern)
		if pattern == "" {
			errs[format.ID] = "匹配表达式为空"
			continue
		}
		re, err := regexp.Compile("^(?:" + pattern + ")")
		if err != nil {
			errs[format.ID] = err.Error()
			continue
		}
		anchors = append(anchors, re)
	}
	return anchors, errs
}

func compileLogConfig(enabled, anchorOnly bool, rules []logRule, anchors []*regexp.Regexp) (*logConfig, map[int64]string) {
	cfg := &logConfig{enabled: enabled, anchorOnly: anchorOnly, anchors: anchors}
	errs := map[int64]string{}
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		compiled := compiledLogRule{
			id: rule.ID, color: rule.Color, bold: rule.Bold,
			whole: rule.Whole, keyword: rule.Keyword,
		}
		if compiled.color < 0 || compiled.color > 15 {
			compiled.color = 7
		}
		if strings.TrimSpace(rule.Pattern) == "" {
			errs[rule.ID] = "匹配表达式为空"
			continue
		}
		if rule.Keyword {
			compiled.kw = []byte(rule.Pattern)
		} else {
			re, err := regexp.Compile(rule.Pattern)
			if err != nil {
				errs[rule.ID] = err.Error()
				continue
			}
			compiled.re = re
		}
		cfg.rules = append(cfg.rules, compiled)
	}
	return cfg, errs
}

type highlightSpan struct {
	start, end int
	color      int
	bold       bool
}

func (c *logConfig) spans(line []byte) []highlightSpan {
	var spans []highlightSpan
	for _, rule := range c.rules {
		if len(spans) >= maxLogSpans {
			break
		}
		if rule.whole {
			matched := false
			if rule.keyword {
				matched = len(rule.kw) > 0 && bytes.Contains(line, rule.kw)
			} else {
				matched = rule.re != nil && rule.re.Match(line)
			}
			if matched {
				spans = addWholeLineSpans(spans, len(line), rule.color, rule.bold)
			}
			continue
		}
		if rule.keyword {
			if len(rule.kw) == 0 {
				continue
			}
			from := 0
			for len(spans) < maxLogSpans {
				i := bytes.Index(line[from:], rule.kw)
				if i < 0 {
					break
				}
				start := from + i
				end := start + len(rule.kw)
				addLogSpan(&spans, start, end, rule.color, rule.bold)
				from = end
			}
			continue
		}
		if rule.re == nil {
			continue
		}
		for _, m := range rule.re.FindAllIndex(line, maxLogSpans) {
			if len(spans) >= maxLogSpans {
				break
			}
			addLogSpan(&spans, m[0], m[1], rule.color, rule.bold)
		}
	}
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].start == spans[j].start {
			return spans[i].end < spans[j].end
		}
		return spans[i].start < spans[j].start
	})
	return spans
}

func addLogSpan(spans *[]highlightSpan, start, end int, color int, bold bool) {
	if start >= end {
		return
	}
	for _, span := range *spans {
		if start < span.end && end > span.start {
			return
		}
	}
	*spans = append(*spans, highlightSpan{start: start, end: end, color: color, bold: bold})
}

// addWholeLineSpans colors the parts of the line earlier rules left alone,
// so a whole-line rule placed later still fills the gaps.
func addWholeLineSpans(spans []highlightSpan, length, color int, bold bool) []highlightSpan {
	if length <= 0 {
		return spans
	}
	ordered := append([]highlightSpan(nil), spans...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].start < ordered[j].start })
	pos := 0
	add := func(start, end int) {
		if start < end && len(spans) < maxLogSpans {
			spans = append(spans, highlightSpan{start: start, end: end, color: color, bold: bold})
		}
	}
	for _, span := range ordered {
		if span.start > pos {
			add(pos, span.start)
		}
		if span.end > pos {
			pos = span.end
		}
	}
	add(pos, length)
	return spans
}

func appendHighlighted(dst, line []byte, spans []highlightSpan) []byte {
	pos := 0
	for _, span := range spans {
		if span.start < pos || span.start >= span.end {
			continue
		}
		dst = append(dst, line[pos:span.start]...)
		dst = append(dst, "\x1b[38;5;"...)
		dst = strconv.AppendInt(dst, int64(span.color), 10)
		dst = append(dst, 'm')
		if span.bold {
			dst = append(dst, "\x1b[1m"...)
		}
		dst = append(dst, line[span.start:span.end]...)
		dst = append(dst, "\x1b[22m\x1b[39m"...)
		pos = span.end
	}
	return append(dst, line[pos:]...)
}

// logHighlighter holds the rules every SSH terminal reads. A connection's
// own state stays in its highlightConn, so sessions do not share parsing.
type logHighlighter struct {
	cfg atomic.Pointer[logConfig]
}

func (h *logHighlighter) store(cfg *logConfig) {
	if h != nil {
		h.cfg.Store(cfg)
	}
}

func (h *logHighlighter) load() *logConfig {
	if h == nil {
		return nil
	}
	return h.cfg.Load()
}

type logHighlightState struct {
	alt   bool
	clean bool
	tail  [8]byte
	scan  []byte
}

var (
	altEnterSeqs = [][]byte{
		[]byte("\x1b[?1049h"),
		[]byte("\x1b[?1047h"),
		[]byte("\x1b[?47h"),
	}
	altExitSeqs = [][]byte{
		[]byte("\x1b[?1049l"),
		[]byte("\x1b[?1047l"),
		[]byte("\x1b[?47l"),
	}
)

func (s *logHighlightState) process(cfg *logConfig, p []byte, out []byte) []byte {
	wasAlt := s.alt
	s.scanAlt(p)
	if s.alt {
		out = append(out, p...)
		s.clean = false
		return out
	}
	exitedAlt := wasAlt
	start := 0
	for i := 0; i < len(p); i++ {
		if p[i] != '\n' {
			continue
		}
		out = s.segment(cfg, p[start:i+1], out)
		start = i + 1
	}
	if start < len(p) {
		out = s.segment(cfg, p[start:], out)
	}
	if exitedAlt {
		// A program leaves the alternate screen with its own redraw:
		// whatever follows starts a fresh line.
		s.clean = true
	}
	return out
}

func (s *logHighlightState) scanAlt(p []byte) {
	s.scan = append(s.scan[:0], s.tail[:]...)
	s.scan = append(s.scan, p...)
	state := s.alt
	last := -1
	set := func(seq []byte, value bool) {
		if i := bytes.LastIndex(s.scan, seq); i > last {
			last, state = i, value
		}
	}
	for _, seq := range altEnterSeqs {
		set(seq, true)
	}
	for _, seq := range altExitSeqs {
		set(seq, false)
	}
	s.alt = state
	if len(s.scan) >= len(s.tail) {
		copy(s.tail[:], s.scan[len(s.scan)-len(s.tail):])
	} else {
		s.tail = [8]byte{}
		copy(s.tail[len(s.tail)-len(s.scan):], s.scan)
	}
}

func (s *logHighlightState) segment(cfg *logConfig, seg []byte, out []byte) []byte {
	hasNL := len(seg) > 0 && seg[len(seg)-1] == '\n'
	text := seg
	if hasNL {
		text = seg[:len(seg)-1]
	}
	// A trailing CR is the CR of a CRLF line ending, not a progress redraw.
	hasCR := len(text) > 0 && text[len(text)-1] == '\r'
	match := text
	if hasCR {
		match = text[:len(text)-1]
	}
	if bytes.IndexByte(match, '\x1b') >= 0 || bytes.IndexByte(match, '\r') >= 0 {
		s.clean = false
	}

	colored := false
	if s.clean && cfg != nil && cfg.enabled && len(match) > 0 && len(match) <= maxLogLineBytes {
		if !cfg.anchorOnly || cfg.isLogLine(match) {
			if spans := cfg.spans(match); len(spans) > 0 {
				out = appendHighlighted(out, match, spans)
				colored = true
			}
		}
	}
	if !colored {
		out = append(out, match...)
	}
	if hasCR {
		out = append(out, '\r')
	}
	if hasNL {
		out = append(out, '\n')
		s.clean = true
	}
	return out
}

// highlightConn colors a terminal's output before the VT sees it. Every
// connection owns its parser state, so sessions do not interfere.
type highlightConn struct {
	src   io.ReadWriteCloser
	hl    *logHighlighter
	state logHighlightState
	in    []byte
	out   []byte
	err   error
}

func newHighlightConn(src io.ReadWriteCloser, hl *logHighlighter) *highlightConn {
	return &highlightConn{
		src:   src,
		hl:    hl,
		state: logHighlightState{clean: true},
		in:    make([]byte, 32<<10),
	}
}

func (c *highlightConn) Read(p []byte) (int, error) {
	for {
		if len(c.out) > 0 {
			n := copy(p, c.out)
			c.out = c.out[n:]
			return n, nil
		}
		if c.err != nil {
			err := c.err
			c.err = nil
			return 0, err
		}
		n, err := c.src.Read(c.in)
		if n > 0 {
			c.out = c.state.process(c.hl.load(), c.in[:n], c.out[:0])
		}
		if err != nil {
			// Keep the error until the bytes just read are delivered.
			c.err = err
		}
		if n == 0 && err == nil {
			return 0, nil
		}
	}
}

func (c *highlightConn) Write(p []byte) (int, error) { return c.src.Write(p) }
func (c *highlightConn) Close() error                { return c.src.Close() }

func (c *highlightConn) Resize(cols, rows int) error {
	if r, ok := c.src.(interface{ Resize(int, int) error }); ok {
		return r.Resize(cols, rows)
	}
	return nil
}

type logHighlightSettings struct {
	Enabled    bool        `json:"enabled"`
	AnchorOnly bool        `json:"anchor_only"`
	Preset     string      `json:"preset"`
	Rules      []logRule   `json:"rules"`
	Formats    []logFormat `json:"formats"`
}

func (a *App) loadLogHighlight(raw string) {
	var saved logHighlightSettings
	if json.Unmarshal([]byte(raw), &saved) != nil {
		return
	}
	a.settings.LogHighlight = saved.Enabled
	a.settings.LogAnchorOnly = saved.AnchorOnly
	if saved.Preset != "" {
		a.logPresetPick = saved.Preset
	}
	if saved.Rules != nil {
		a.settings.LogRules = saved.Rules
	}
	if saved.Formats != nil {
		a.settings.LogFormats = saved.Formats
	}
}

func (a *App) logHighlightValue() string {
	data, err := json.Marshal(logHighlightSettings{
		Enabled:    a.settings.LogHighlight,
		AnchorOnly: a.settings.LogAnchorOnly,
		Preset:     a.logPresetPick,
		Rules:      a.settings.LogRules,
		Formats:    a.settings.LogFormats,
	})
	if err != nil {
		return ""
	}
	return string(data)
}

func (a *App) applyLogHighlight() {
	anchors, formatErrs := compileLogFormats(a.settings.LogFormats)
	cfg, errs := compileLogConfig(a.settings.LogHighlight, a.settings.LogAnchorOnly, a.settings.LogRules, anchors)
	a.logRuleErrs = errs
	a.logFormatErrs = formatErrs
	if a.logHL == nil {
		a.logHL = &logHighlighter{}
	}
	a.logHL.store(cfg)
}
