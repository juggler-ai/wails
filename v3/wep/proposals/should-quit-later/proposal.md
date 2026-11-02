# Wails Enhancement Proposal (WEP)

## Deciding a quit later

**WEP Number**: (leave blank, assigned on acceptance)  
**Status**: Draft  
**Author**: Julian Storer  
**Created**: 2026-11-02  
**Discussion**: none found (searched issues and PRs for `ShouldQuit`, `NSTerminateLater`, `applicationShouldTerminate`)  
**Implementor**: Julian Storer  
**Target**: Wails v3

## Summary

Add `Options.ShouldQuitLater func(decide func(quit bool))`, a form of
`ShouldQuit` that answers a request to quit asynchronously. While the decision
is awaited the request is held open: on macOS the application replies
`NSTerminateLater`, so a quit the system sends at log out, restart or shut
down waits for the application instead of being cancelled. The default, unset,
is today's behaviour.

## Motivation

`ShouldQuit` must answer synchronously, on the main thread. An application
that needs to finish something before it can say yes — save documents, flush a
database, stop child processes, wait for an in-flight network request, or ask
the user in a dialog whose answer arrives through a callback — has two options
today, and both are poor:

1. **Block in `ShouldQuit`.** The main thread stops: windows stop painting, and
   any work the cleanup itself dispatches to the main thread (`InvokeSync`,
   window or dialog calls) deadlocks.
2. **Return `false`, do the work, then call `app.Quit()` again.** This is the
   usual workaround, and it works for Cmd+Q. But on macOS, returning `false`
   replies `NSTerminateCancel`, and when the request came from the system — log
   out, restart, shut down, or another application sending a quit Apple Event —
   cancelling it cancels the log out. The user sees "*App* interrupted logging
   out", and the log out has to be started again once the application has
   quit by itself.

AppKit has a reply made for exactly this case: `NSTerminateLater`, answered
later with `replyToApplicationShouldTerminate:`. Wails gives applications no way
to use it.

## Detailed Design

### API

```go
type Options struct {
    // ...

    // ShouldQuitLater is ShouldQuit for an application that decides
    // asynchronously, and is used in its place when set. It is called on the
    // main thread when something asks the application to quit, and must
    // eventually call decide exactly once, from any goroutine: decide(true)
    // quits, decide(false) does not. While the decision is awaited, a further
    // request to quit joins the one held open, and a call to App.Quit counts as
    // decide(true).
    //
    // On macOS the request is held open until then (NSTerminateLater), so a quit
    // the system sends at log out, restart or shut down waits for the decision,
    // where ShouldQuit returning false cancels it and with it the log out.
    ShouldQuitLater func(decide func(quit bool))
}
```

### Semantics

- **Precedence:** when `ShouldQuitLater` is set, `ShouldQuit` is not called.
- **Deciding at once:** calling `decide` before `ShouldQuitLater` returns answers
  the request immediately, exactly as `ShouldQuit` returning the same value
  would. An application that only sometimes needs time pays nothing in the
  common case.
- **One decision per request:** only the first call to `decide` counts; later
  calls are ignored.
- **Joining:** a request made while one is held open (a second Cmd+Q, a second
  `app.Quit()` on Windows or Linux) does not call `ShouldQuitLater` again; the
  pending decision answers it.
- **`app.Quit()`:** while a decision is awaited, `app.Quit()` decides it yes.
  This is the natural way for the work started by `ShouldQuitLater` to finish:
  an application that already ends its shutdown with `app.Quit()` needs no
  extra plumbing to deliver the answer.
- **On yes:** the usual shutdown runs (`OnShutdown`, service shutdown, windows
  closed) on the main thread, then the application exits, as with `ShouldQuit`.

### Implementation

A platform-neutral `askShouldQuit(later func(bool)) quitReply` holds the
logic: with no `ShouldQuitLater` it calls `shouldQuit()` and replies now or
cancel; otherwise it records a pending request, calls `ShouldQuitLater`, and
replies now, cancel, or later depending on whether `decide` was called before
it returned. A later decision is passed to the platform's `later` callback.
`App.Quit` first answers a pending request yes, if there is one.

- **macOS (cgo):** `applicationShouldTerminate:` maps the reply to
  `NSTerminateCancel`, `NSTerminateNow` or `NSTerminateLater`. A later decision
  runs on the main thread: on yes, `cleanup()` and then
  `[NSApp replyToApplicationShouldTerminate:YES]`; on no, the reply alone.
  `shouldQuitApplication` returns the reply as an `int` instead of a `bool`.
- **macOS (purego):** the same, through the purego delegate.
- **Windows and Linux:** `destroy()` asks the same way. A yes, now or later,
  runs the existing shutdown (`cleanup()`, then destroying the main thread
  window or the GTK application) on the main thread.

### Impact on existing functionality

None when unset: `askShouldQuit` reduces to today's `shouldQuit()` check, and
each platform's quit path takes the same branch it does today.

## Non-Goals

- Changing `ShouldQuit`. Its synchronous contract, and its cancelling reply on
  macOS, stay as they are.
- A time limit on the decision. macOS applies its own at log out; an
  application that wants one can call `decide` from a timer.
- Distinguishing the source of a request (user, system, `app.Quit()`) to the
  callback. That is a separate question, and would apply equally to
  `ShouldQuit`.
- Windows' `WM_QUERYENDSESSION`, which is answered synchronously and asks
  whether the session may end at all. That needs its own design.

## Platform Considerations

- **macOS:** the only platform with a native deferred reply. While a
  request is held open AppKit runs its event loop in modal panel mode, as it
  does for any application replying `NSTerminateLater`; dialogs, and work
  dispatched to the main queue, continue to run.
- **Windows and Linux:** neither has a deferred reply to the system. The
  callback still lets the application decide without blocking the main
  thread, and the same code works on all three platforms. At session end,
  Windows ends the process once `WM_ENDSESSION` returns, deferred or not, as it
  does today.
- **iOS and Android:** unchanged; their quit paths do not consult `ShouldQuit`
  today and do not consult this either.

## Pros/Cons

**Pros**

- Logging out, restarting or shutting down no longer gets cancelled by an
  application that needs a moment to clean up.
- No main-thread blocking, and no deadlock with work the cleanup dispatches to
  the main thread.
- Opt-in and additive: no change for applications that do not set it.

**Cons**

- A second way to answer the same question. The docs present `ShouldQuitLater`
  as the form to use when the answer is not immediate.
- An application that never calls `decide` keeps the request open
  indefinitely, as AppKit does for an application that never replies.

## Alternatives Considered

- **Make `ShouldQuit` asynchronous.** A breaking change to a widely used
  option.
- **Return a tri-state from `ShouldQuit`** (now/cancel/later), with a separate
  `App.ReplyToQuit(bool)`. Splits one decision across two APIs, and leaves the
  reply unconnected to the request it answers.
- **A macOS-only option** (`MacOptions.TerminateLater`). The need — deciding
  without blocking — is not macOS-specific, and a cross-platform callback keeps
  application code the same everywhere.
- **Keep returning `false` and calling `app.Quit()` again.** The current
  workaround; it is what cancels the log out.

## Backwards Compatibility

Additive. Applications that do not set `ShouldQuitLater` see no change.
Setting both `ShouldQuit` and `ShouldQuitLater` uses `ShouldQuitLater`.

## Security and Privacy

None.

## Test Plan

- **Unit tests** (platform-neutral, `quit_decision_test.go`): `ShouldQuit`
  answered at once; `ShouldQuitLater` decided at once, both ways; a request held
  open until decided, with only the first decision counting; a request while
  one is held open joining it; `App.Quit` answering a pending request.
- **Manual, macOS:** an application whose `ShouldQuitLater` finishes its work
  on a goroutine and then calls `app.Quit()`. A quit Apple Event sent with a
  reply awaited — as `loginwindow` sends at log out — is answered with no error
  once the work is done, where the same application using `ShouldQuit`
  answers `-128` (`userCanceledErr`) at once.
- **Build:** the package compiles for macOS (cgo and purego), Windows and
  Linux.

## Reference Implementation

The fork's `feat/should-quit-later` branch, on current `master`, implements
this proposal with its unit tests and documentation.

## Maintenance Plan

The logic is in one platform-neutral file with its tests; each platform's
change is a few lines in its existing quit path. The implementor will maintain
it.

## Conclusion

`ShouldQuitLater` lets an application decide a quit without blocking the main
thread and, on macOS, without cancelling a log out, restart or shut down. It is
small, opt-in, and maps directly onto the reply AppKit already provides for the
purpose.
