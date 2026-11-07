package app

import (
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

func (a *App) openFind() {
	if a.activeTerminal() == nil {
		return
	}
	a.finding = true
	a.resetFind()
}

func (a *App) closeFind() {
	if ts := a.activeTerminal(); ts != nil && ts.Term != nil && a.lastSearch != "" {
		ts.Term.EndSearch()
	}
	a.finding = false
	a.resetFind()
}

func (a *App) resetFind() {
	a.query = ""
	a.lastSearch = ""
	a.searchTotal = 0
	a.searchIndex = 0
	a.lastIndex = 0
	a.searchInit = true
}

func (a *App) onTermSearch(termID int64, s terminal.SearchStatus) {
	ts := a.activeTerminal()
	if ts == nil || ts.ID != termID || !a.finding {
		return
	}

	a.searchTotal = s.Total
	if !a.searchInit {
		idx := s.Selected - 1 // SearchStatus.Selected is 1-based
		if idx < 0 {
			idx = 0
		}
		if s.Total > 0 && idx >= s.Total {
			idx = s.Total - 1
		}
		a.searchIndex = idx
		a.lastIndex = idx
		a.searchInit = true
	}
}

func (a *App) findBar(c *ui.Context, ts *termSession) {
	if a.query != a.lastSearch {
		a.lastSearch = a.query
		a.searchTotal = 0
		a.searchIndex = 0
		a.lastIndex = 0
		a.searchInit = false
		if a.query == "" {
			ts.Term.EndSearch()
		} else {
			ts.Term.Search(a.query)
		}
	}

	if a.searchTotal > 0 {
		if a.searchIndex < 0 {
			a.searchIndex = 0
		}
		if a.searchIndex >= a.searchTotal {
			a.searchIndex = a.searchTotal - 1
		}
	} else {
		a.searchIndex = 0
	}

	ui.FindBar(c, &a.finding, &a.query, a.searchTotal, &a.searchIndex)

	if a.searchIndex == a.lastIndex {
		return
	}

	prev, next := a.lastIndex, a.searchIndex
	switch {
	case a.searchTotal > 1 && prev == a.searchTotal-1 && next == 0:
		ts.Term.SearchNext()
	case a.searchTotal > 1 && prev == 0 && next == a.searchTotal-1:
		ts.Term.SearchPrevious()
	case next > prev:
		for i := prev; i < next; i++ {
			ts.Term.SearchNext()
		}
	default:
		for i := next; i < prev; i++ {
			ts.Term.SearchPrevious()
		}
	}
	a.lastIndex = next
}
