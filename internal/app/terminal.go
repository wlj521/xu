package app

import (
	"io"
	"sync"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
	"golang.org/x/crypto/ssh"

	"xu/internal/sshx"
)

// sshTermConn adapts an SSH session to the terminal plugin's Conn.
type sshTermConn struct {
	sess *ssh.Session
	in   io.WriteCloser
	out  io.Reader
	once sync.Once
	err  error
}

func newSSHTermConn(client *ssh.Client, cols, rows int) (*sshTermConn, error) {
	sess, err := client.NewSession()
	if err != nil {
		return nil, err
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := sess.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		sess.Close()
		return nil, err
	}

	in, err := sess.StdinPipe()
	if err != nil {
		sess.Close()
		return nil, err
	}
	out, err := sess.StdoutPipe()
	if err != nil {
		sess.Close()
		return nil, err
	}
	sess.Stderr = io.Discard

	if err := sess.Shell(); err != nil {
		sess.Close()
		return nil, err
	}

	return &sshTermConn{sess: sess, in: in, out: out}, nil
}

func (c *sshTermConn) Read(p []byte) (int, error)  { return c.out.Read(p) }
func (c *sshTermConn) Write(p []byte) (int, error) { return c.in.Write(p) }

func (c *sshTermConn) Close() error {
	c.once.Do(func() { c.err = c.sess.Close() })
	return c.err
}

// Resize is discovered by the terminal plugin through an interface check.
// SSH's WindowChange takes (height, width).
func (c *sshTermConn) Resize(cols, rows int) error {
	return c.sess.WindowChange(rows, cols)
}

type termSession struct {
	ID           int64
	Title        string
	Term         *terminal.Terminal
	conn         *sshTermConn
	sess         *ssh.Session
	client       *ssh.Client
	profile      sshx.Profile
	profileID    int64
	local        bool
	exited       bool
	closing      bool
	reconnecting bool
	reconnectErr string
}

type termDisplay struct {
	Font         terminal.Font
	Scrollback   int
	CopyOnSelect bool
	Light        *terminal.Theme
	Dark         *terminal.Theme
}

func newTermSession(client *ssh.Client, p sshx.Profile, cols, rows int, post func(func()), onSearch func(int64, terminal.SearchStatus), display termDisplay) (*termSession, error) {
	conn, err := newSSHTermConn(client, cols, rows)
	if err != nil {
		return nil, err
	}

	ts := &termSession{
		ID:      time.Now().UnixNano(),
		Title:   p.Username + "@" + p.Host,
		conn:    conn,
		sess:    conn.sess,
		client:  client,
		profile: p,
	}

	term, err := terminal.New(terminal.Options{
		Conn:         conn,
		Scrollback:   display.Scrollback,
		Font:         display.Font,
		CopyOnSelect: copyOnSelect(display),
		Theme:        display.Light,
		DarkTheme:    display.Dark,
		OnTitle: func(title string) {
			post(func() { ts.Title = title })
		},
		OnExit: func(code int) {
			post(func() {
				if !ts.closing {
					ts.exited = true
				}
			})
		},
		OnSearch: func(s terminal.SearchStatus) {
			if onSearch != nil {
				post(func() { onSearch(ts.ID, s) })
			}
		},
	})
	if err != nil {
		conn.Close()
		client.Close()
		return nil, err
	}

	ts.Term = term
	return ts, nil
}

func terminalView(c *ui.Context, ts *termSession) {
	if ts.Term == nil {
		return
	}
	terminal.View(c, ts.Term).Grow(1).FillWidth().AutoFocus()
}

func (t *termSession) Close() {
	t.closing = true
	if t.Term != nil {
		t.Term.Close()
	}
	if t.sess != nil {
		t.sess.Close()
	}
	if t.client != nil {
		t.client.Close()
	}
}
