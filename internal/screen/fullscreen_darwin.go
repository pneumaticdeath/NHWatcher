package screen

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <AppKit/AppKit.h>

static void getScreenSize(int *w, int *h) {
    NSScreen *screen = [NSScreen mainScreen];
    NSRect frame = [screen frame];
    *w = (int)frame.size.width;
    *h = (int)frame.size.height;
}

// hideNSWindow makes the window invisible while keeping it renderable.
// We set alpha=0 and level below desktop so it stays behind everything.
// Canvas.Capture() works on the backing store regardless of alpha.
// Must be called on the main thread (via fyne.Do).
static void hideNSWindow(void *nswindow) {
    NSWindow *win = (__bridge NSWindow *)nswindow;
    [win setAlphaValue:0.0];
    [win setLevel:kCGDesktopWindowLevelKey - 1];
}

*/
import "C"
import "unsafe"

// ScreenSize returns the main display dimensions in points.
func ScreenSize() (int, int) {
	var w, h C.int
	C.getScreenSize(&w, &h)
	return int(w), int(h)
}

// HideNSWindow makes the window invisible while keeping Canvas.Capture working.
// Used in screensaver mode where the ObjC view draws piped frames.
// Must be called on the main thread (via fyne.Do).
func HideNSWindow(nswindow uintptr) {
	C.hideNSWindow(unsafe.Pointer(nswindow))
}

