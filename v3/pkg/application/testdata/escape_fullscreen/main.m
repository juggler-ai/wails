#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#include <assert.h>
#include <stdio.h>
#include <string.h>
#include "webview_window_darwin.h"

// Exercises windowDropsEscapeCommand, the decision WebviewWindow and
// WebviewPanel make in doCommandBySelector: for DisableEscapeExitsFullscreen.
//
//   --filter                 decide against probe windows; needs no display.
//   --fullscreen [--allow]   a real fullscreen window holding a WKWebView with a
//                            focused <textarea> receives Escape. Without
//                            --allow the window must stay fullscreen; with it
//                            (the option off) it must leave, which is what
//                            proves the press reached the path being guarded.
//                            Either way the page must see the keydown, and
//                            toggleFullScreen: must still leave fullscreen.
// Exits non-zero on any failure, and within 20 seconds whatever happens.

static BOOL allowEscape = NO;

// A window as far as windowDropsEscapeCommand reads one.
@interface ProbeWindow : NSObject
@property BOOL disableEscapeExitsFullscreen;
@property NSWindowStyleMask styleMask;
@property (assign) id delegate;
@end
@implementation ProbeWindow
@end

@interface CancellingDelegate : NSObject
- (void)cancelOperation:(id)sender;
@end
@implementation CancellingDelegate
- (void)cancelOperation:(id)sender {}
@end

static BOOL drops(ProbeWindow *window, SEL selector) {
    return windowDropsEscapeCommand((NSWindow<WailsWebviewWindow> *)window, selector);
}

static void testFilter(void) {
    ProbeWindow *window = [ProbeWindow new];
    window.disableEscapeExitsFullscreen = YES;
    window.styleMask = NSWindowStyleMaskTitled | NSWindowStyleMaskFullScreen;
    assert(drops(window, @selector(cancelOperation:)));
    assert(drops(window, @selector(cancel:)));
    // Every other command still reaches NSWindow.
    assert(!drops(window, @selector(insertNewline:)));
    assert(!drops(window, @selector(complete:)));
    // Only while fullscreen, and only when asked for.
    window.styleMask = NSWindowStyleMaskTitled;
    assert(!drops(window, @selector(cancelOperation:)));
    window.styleMask = NSWindowStyleMaskTitled | NSWindowStyleMaskFullScreen;
    window.disableEscapeExitsFullscreen = NO;
    assert(!drops(window, @selector(cancelOperation:)));
    // A delegate that handles the command still gets it.
    window.disableEscapeExitsFullscreen = YES;
    CancellingDelegate *delegate = [CancellingDelegate new];
    window.delegate = delegate;
    assert(!drops(window, @selector(cancelOperation:)));
}

// The window classes' override, verbatim.
@interface GuardedWindow : NSWindow <WailsWebviewWindow>
@property (assign) WKWebView *webView;
@property BOOL disableEscapeExitsFullscreen;
@end
@implementation GuardedWindow
- (BOOL)canBecomeKeyWindow {
    return YES;
}
- (void)doCommandBySelector:(SEL)selector {
    if (windowDropsEscapeCommand(self, selector)) {
        return;
    }
    [super doCommandBySelector:selector];
}
@end

@interface Driver : NSObject <NSApplicationDelegate, NSWindowDelegate, WKScriptMessageHandler>
@property (retain) GuardedWindow *window;
@property BOOL pageSawEscape;
@property BOOL leaving;
@end

static BOOL isFullscreen(NSWindow *window) {
    return (window.styleMask & NSWindowStyleMaskFullScreen) == NSWindowStyleMaskFullScreen;
}

static void fail(const char *message) {
    fprintf(stderr, "FAIL: %s\n", message);
    exit(1);
}

static void after(double seconds, dispatch_block_t block) {
    dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(seconds * NSEC_PER_SEC)), dispatch_get_main_queue(), block);
}

@implementation Driver
- (void)userContentController:(WKUserContentController *)controller didReceiveScriptMessage:(WKScriptMessage *)message {
    if ([[message.body description] isEqualToString:@"Escape"]) {
        self.pageSawEscape = YES;
    }
}

- (void)applicationDidFinishLaunching:(NSNotification *)notification {
    WKWebViewConfiguration *config = [WKWebViewConfiguration new];
    [config.userContentController addScriptMessageHandler:self name:@"keydown"];
    NSRect frame = NSMakeRect(200, 200, 640, 400);
    self.window = [[GuardedWindow alloc] initWithContentRect:frame
                                                   styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskResizable
                                                     backing:NSBackingStoreBuffered
                                                       defer:NO];
    self.window.releasedWhenClosed = NO;
    self.window.collectionBehavior = NSWindowCollectionBehaviorFullScreenPrimary;
    self.window.disableEscapeExitsFullscreen = !allowEscape;
    self.window.delegate = self;
    WKWebView *webView = [[WKWebView alloc] initWithFrame:frame configuration:config];
    self.window.webView = webView;
    self.window.contentView = webView;
    [webView loadHTMLString:@"<textarea autofocus>draft</textarea><script>"
        "addEventListener('load', () => document.querySelector('textarea').focus());"
        "addEventListener('keydown', e => window.webkit.messageHandlers.keydown.postMessage(e.key));"
        "</script>" baseURL:nil];
    [NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
    [self.window makeKeyAndOrderFront:nil];
    [NSApp activateIgnoringOtherApps:YES];
    after(1.0, ^{
        [self.window makeFirstResponder:webView];
        [self.window toggleFullScreen:nil];
    });
    after(20.0, ^{
        fail("timed out");
    });
}

- (void)windowDidEnterFullScreen:(NSNotification *)notification {
    after(1.0, ^{
        [self pressEscape];
    });
}

- (void)pressEscape {
    NSInteger number = self.window.windowNumber;
    NSTimeInterval now = [[NSProcessInfo processInfo] systemUptime];
    for (NSEventType type = NSEventTypeKeyDown; type <= NSEventTypeKeyUp; type++) {
        NSEvent *event = [NSEvent keyEventWithType:type
                                          location:NSZeroPoint
                                     modifierFlags:0
                                         timestamp:now
                                      windowNumber:number
                                           context:nil
                                        characters:@"\x1b"
                       charactersIgnoringModifiers:@"\x1b"
                                         isARepeat:NO
                                           keyCode:53];
        [NSApp postEvent:event atStart:NO];
    }
    after(2.0, ^{
        if (!self.pageSawEscape) {
            fail("the page did not receive the Escape keydown");
        }
        if (allowEscape) {
            if (isFullscreen(self.window)) {
                fail("Escape did not leave fullscreen with the option off, so the guarded path was never reached");
            }
            exit(0);
        }
        if (!isFullscreen(self.window)) {
            fail("Escape left fullscreen");
        }
        self.leaving = YES;
        [self.window toggleFullScreen:nil];
    });
}

- (void)windowDidExitFullScreen:(NSNotification *)notification {
    if (self.leaving) {
        exit(0);
    }
}
@end

int main(int argc, const char **argv) {
    @autoreleasepool {
        BOOL fullscreen = NO;
        for (int i = 1; i < argc; i++) {
            if (strcmp(argv[i], "--filter") == 0) {
                testFilter();
                return 0;
            }
            if (strcmp(argv[i], "--fullscreen") == 0) {
                fullscreen = YES;
            }
            if (strcmp(argv[i], "--allow") == 0) {
                allowEscape = YES;
            }
        }
        if (!fullscreen) {
            fprintf(stderr, "usage: %s --filter | --fullscreen [--allow]\n", argv[0]);
            return 2;
        }
        NSApplication *app = [NSApplication sharedApplication];
        Driver *driver = [Driver new];
        app.delegate = driver;
        [app run];
    }
    return 0;
}
