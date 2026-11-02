package application

import (
	"sync/atomic"
	"testing"
)

// answers records the answers a quit request's later callback is given.
type answers struct{ yes, no atomic.Int32 }

func (r *answers) later(quit bool) {
	if quit {
		r.yes.Add(1)
	} else {
		r.no.Add(1)
	}
}

func TestShouldQuitAnswersAtOnce(t *testing.T) {
	for _, quit := range []bool{true, false} {
		app := &App{options: Options{ShouldQuit: func() bool { return quit }}}
		var r answers
		want := quitCancel
		if quit {
			want = quitNow
		}
		if got := app.askShouldQuit(r.later); got != want {
			t.Errorf("ShouldQuit returning %v: reply %v, want %v", quit, got, want)
		}
		if r.yes.Load()+r.no.Load() != 0 {
			t.Errorf("ShouldQuit returning %v: the later callback was called", quit)
		}
	}
}

func TestShouldQuitLaterDecidedAtOnceAnswersAtOnce(t *testing.T) {
	for _, quit := range []bool{true, false} {
		app := &App{options: Options{ShouldQuitLater: func(decide func(bool)) { decide(quit) }}}
		var r answers
		want := quitCancel
		if quit {
			want = quitNow
		}
		if got := app.askShouldQuit(r.later); got != want {
			t.Errorf("deciding %v at once: reply %v, want %v", quit, got, want)
		}
		if r.yes.Load()+r.no.Load() != 0 {
			t.Errorf("deciding %v at once: the later callback was called as well", quit)
		}
		if app.answerPendingQuit(true) {
			t.Errorf("deciding %v at once left the request pending", quit)
		}
	}
}

func TestShouldQuitLaterHoldsTheRequestUntilDecided(t *testing.T) {
	var decide func(bool)
	app := &App{options: Options{ShouldQuitLater: func(d func(bool)) { decide = d }}}
	var r answers
	if got := app.askShouldQuit(r.later); got != quitLater {
		t.Fatalf("an undecided request: reply %v, want %v", got, quitLater)
	}
	if r.yes.Load()+r.no.Load() != 0 {
		t.Fatal("the later callback was called before a decision")
	}
	decide(true)
	decide(false)
	if r.yes.Load() != 1 || r.no.Load() != 0 {
		t.Errorf("deciding yes and then no: %d yes, %d no answers, want the first decision only", r.yes.Load(), r.no.Load())
	}
	if app.answerPendingQuit(true) {
		t.Error("a decided request is still pending")
	}
}

func TestARequestWhileOneIsHeldOpenJoinsIt(t *testing.T) {
	var asked atomic.Int32
	var decide func(bool)
	app := &App{options: Options{ShouldQuitLater: func(d func(bool)) {
		asked.Add(1)
		decide = d
	}}}
	var first, second answers
	if got := app.askShouldQuit(first.later); got != quitLater {
		t.Fatalf("an undecided request: reply %v, want %v", got, quitLater)
	}
	if got := app.askShouldQuit(second.later); got != quitLater {
		t.Fatalf("a request while one is held open: reply %v, want %v", got, quitLater)
	}
	if n := asked.Load(); n != 1 {
		t.Errorf("ShouldQuitLater was asked %d times for two requests while one was held open, want 1", n)
	}
	decide(true)
	if first.yes.Load() != 1 || second.yes.Load()+second.no.Load() != 0 {
		t.Errorf("one decision: first request %d yes, second %d answers; want the decision delivered once, through the first", first.yes.Load(), second.yes.Load()+second.no.Load())
	}
	if got := app.askShouldQuit(second.later); got != quitLater || asked.Load() != 2 {
		t.Errorf("a request after the held one was decided: reply %v after %d asks, want a new decision asked for", got, asked.Load())
	}
}

func TestQuitAnswersAPendingRequest(t *testing.T) {
	app := &App{options: Options{ShouldQuitLater: func(func(bool)) {}}}
	if app.answerPendingQuit(true) {
		t.Error("with no quit request pending, one was answered")
	}
	var r answers
	if got := app.askShouldQuit(r.later); got != quitLater {
		t.Fatalf("an undecided request: reply %v, want %v", got, quitLater)
	}
	// impl is nil, so a Quit that did not answer the pending request would
	// do nothing at all.
	app.Quit()
	if r.yes.Load() != 1 || r.no.Load() != 0 {
		t.Errorf("Quit while a request was pending: %d yes, %d no answers, want one yes", r.yes.Load(), r.no.Load())
	}
}
