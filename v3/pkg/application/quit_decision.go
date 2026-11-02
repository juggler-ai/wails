package application

// quitReply is the answer to a request to quit: the platform's reply to it.
type quitReply int

const (
	// quitCancel: the application stays.
	quitCancel quitReply = iota
	// quitNow: the application quits.
	quitNow
	// quitLater: ShouldQuitLater has not decided yet; the decision is passed
	// to the later callback askShouldQuit was given.
	quitLater
)

func (r quitReply) String() string {
	switch r {
	case quitCancel:
		return "cancel"
	case quitNow:
		return "now"
	case quitLater:
		return "later"
	}
	return "unknown"
}

// pendingQuit is a request to quit awaiting ShouldQuitLater's decision.
type pendingQuit struct {
	// answered and quit are the decision, once made.
	answered, quit bool
	// returned is set once askShouldQuit has replied quitLater, after which a
	// decision goes to later rather than to askShouldQuit's reply.
	returned bool
	later    func(quit bool)
}

// askShouldQuit asks the application whether a request to quit goes ahead.
//
// With ShouldQuit, or neither option, the answer is quitNow or quitCancel.
// With ShouldQuitLater, a decision made before it returns is answered the same
// way. Otherwise the request is held open and askShouldQuit replies quitLater:
// the decision is passed to later, once, from whichever goroutine makes it.
// Until then a call to Quit decides it yes (answerPendingQuit), and a further
// request joins it: it replies quitLater, and the one decision answers both,
// through the first request's later.
func (a *App) askShouldQuit(later func(quit bool)) quitReply {
	if a.options.ShouldQuitLater == nil {
		if a.shouldQuit() {
			return quitNow
		}
		return quitCancel
	}
	p := &pendingQuit{}
	a.quitLock.Lock()
	if held := a.pendingQuit; held != nil && held.returned {
		a.quitLock.Unlock()
		return quitLater
	}
	a.pendingQuit = p
	a.quitLock.Unlock()

	a.options.ShouldQuitLater(func(quit bool) { a.decideQuit(p, quit) })

	a.quitLock.Lock()
	defer a.quitLock.Unlock()
	if !p.answered {
		p.returned, p.later = true, later
		return quitLater
	}
	if a.pendingQuit == p {
		a.pendingQuit = nil
	}
	if p.quit {
		return quitNow
	}
	return quitCancel
}

// decideQuit records quit as the decision on p, unless p has one already, and
// passes it to p's later callback once askShouldQuit has replied quitLater.
func (a *App) decideQuit(p *pendingQuit, quit bool) {
	a.quitLock.Lock()
	if p.answered {
		a.quitLock.Unlock()
		return
	}
	p.answered, p.quit = true, quit
	later := p.later
	if p.returned && a.pendingQuit == p {
		a.pendingQuit = nil
	}
	a.quitLock.Unlock()
	if later != nil {
		later(quit)
	}
}

// answerPendingQuit decides a request to quit that awaits ShouldQuitLater's
// decision, and reports whether there was one.
func (a *App) answerPendingQuit(quit bool) bool {
	a.quitLock.Lock()
	p := a.pendingQuit
	a.quitLock.Unlock()
	if p == nil {
		return false
	}
	a.decideQuit(p, quit)
	return true
}
