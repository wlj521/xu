package app

import (
	"errors"

	"golang.org/x/crypto/ssh"

	"xu/internal/sshx"
)

func (a *App) dialWithPrompt(p sshx.Profile) (*ssh.Client, error) {
	client, err := sshx.Dial(a.ctx, p, a.trustHost)

	var hk *sshx.HostKeyError
	if errors.As(err, &hk) {
		if a.confirmHostKey(hk) {
			a.trustMu.Lock()
			a.trusted[hk.Host] = hk.Fingerprint
			a.trustMu.Unlock()
			a.post(func() { a.saveKnownHost(hk) })
			client, err = sshx.Dial(a.ctx, p, a.trustHost)
		}
	}
	return client, err
}

func (a *App) reconnect(ts *termSession) {
	if ts == nil || ts.reconnecting || ts.local {
		return
	}

	ts.reconnecting = true
	ts.reconnectErr = ""
	p := ts.profile

	go func() {
		client, err := a.dialWithPrompt(p)
		if err != nil {
			a.post(func() {
				ts.reconnecting = false
				ts.reconnectErr = err.Error()
			})
			return
		}

		newTS, err := newTermSession(client, p, 100, 30, a.post, a.onTermSearch, a.termDisplay())
		if err != nil {
			client.Close()
			a.post(func() {
				ts.reconnecting = false
				ts.reconnectErr = "重连失败: " + err.Error()
			})
			return
		}
		newTS.profileID = ts.profileID
		newTS.reconnecting = false

		if a.ctx.Err() != nil {
			newTS.Close()
			return
		}

		a.post(func() {
			idx := -1
			for i, t := range a.terms {
				if t == ts {
					idx = i
					break
				}
			}
			if idx < 0 {
				newTS.Close()
				return
			}

			a.terms[idx] = newTS
			if ti := a.termTabIndex(ts); ti >= 0 {
				a.tabs[ti].term = newTS
				a.activeTab = ti
			}
			a.activeTerm = idx

			a.finding = false
			a.resetFind()
			a.filesOpen = false
			a.monitorOpen = false
			a.forwardOpen = false
			a.batchOpen = false

			if a.editor != nil && a.editor.termID == ts.ID {
				a.discardEditor()
			}
			a.stopMonitor(ts.ID)
			a.stopForwards(ts.ID)
			if b := a.browsers[ts.ID]; b != nil {
				b.Close()
				delete(a.browsers, ts.ID)
			}

			ts.Close()
			a.autoStartForwards(newTS)
			a.startMonitorBackground(newTS)
		})
	}()
}
