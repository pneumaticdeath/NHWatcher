#import "NHWatcherView.h"

@implementation NHWatcherView {
    NSTask *_task;
    NSImage *_previewImage;
}

- (instancetype)initWithFrame:(NSRect)frame isPreview:(BOOL)isPreview {
    self = [super initWithFrame:frame isPreview:isPreview];
    if (self) {
        [self setAnimationTimeInterval:1.0/30.0];
        [self setWantsLayer:YES];

        // Pre-load the preview image from the bundle
        NSBundle *bundle = [NSBundle bundleForClass:[self class]];
        NSString *imgPath = [bundle pathForResource:@"preview" ofType:@"png"];
        if (imgPath) {
            _previewImage = [[NSImage alloc] initWithContentsOfFile:imgPath];
        }
    }
    return self;
}

// Returns YES only when running as the actual screensaver (fullscreen,
// high window level), not in any System Settings preview context.
- (BOOL)isActualScreenSaver {
    NSWindow *win = [self window];
    if (!win) return NO;
    return [win level] >= NSScreenSaverWindowLevel;
}

- (void)startAnimation {
    [super startAnimation];

    // Only launch the Go binary for the real screensaver, not any
    // System Settings preview (including full-size hover previews).
    if (![self isActualScreenSaver]) {
        return;
    }

    [self launchViewer];
}

- (void)stopAnimation {
    [super stopAnimation];
    [self terminateViewer];
}

- (void)drawRect:(NSRect)rect {
    [[NSColor blackColor] setFill];
    NSRectFill(rect);

    if ([self isActualScreenSaver]) {
        return;
    }

    // Draw the preview image in all non-screensaver contexts
    // (grid thumbnail, hover preview, etc.)
    NSRect bounds = [self bounds];
    if (bounds.size.width < 1 || bounds.size.height < 1) {
        return;
    }
    if (!_previewImage) {
        return;
    }

    NSSize imgSize = [_previewImage size];
    CGFloat scaleX = bounds.size.width / imgSize.width;
    CGFloat scaleY = bounds.size.height / imgSize.height;
    CGFloat scale = fmax(scaleX, scaleY);
    CGFloat drawW = imgSize.width * scale;
    CGFloat drawH = imgSize.height * scale;
    NSRect drawRect = NSMakeRect(
        (bounds.size.width - drawW) / 2,
        (bounds.size.height - drawH) / 2,
        drawW, drawH
    );
    [_previewImage drawInRect:drawRect
                     fromRect:NSZeroRect
                    operation:NSCompositingOperationSourceOver
                     fraction:1.0];
}

- (void)animateOneFrame {
    if (![self isActualScreenSaver]) {
        [self setNeedsDisplay:YES];
    }
}

- (BOOL)hasConfigureSheet {
    return NO;
}

- (NSWindow *)configureSheet {
    return nil;
}

- (void)launchViewer {
    if (_task && [_task isRunning]) {
        return;
    }

    NSBundle *bundle = [NSBundle bundleForClass:[self class]];
    NSString *binPath = [bundle pathForResource:@"nhwatcher" ofType:nil];

    if (!binPath) {
        NSLog(@"NHWatcher: nhwatcher binary not found in bundle Resources");
        return;
    }

    _task = [[NSTask alloc] init];
    [_task setExecutableURL:[NSURL fileURLWithPath:binPath]];
    [_task setArguments:@[@"--screensaver"]];

    NSPipe *errPipe = [NSPipe pipe];
    [_task setStandardError:errPipe];

    NSError *error = nil;
    [_task launchAndReturnError:&error];
    if (error) {
        NSLog(@"NHWatcher: failed to launch: %@", error);
        _task = nil;
        return;
    }

    NSLog(@"NHWatcher: launched viewer (PID %d)", [_task processIdentifier]);

    dispatch_async(dispatch_get_global_queue(DISPATCH_QUEUE_PRIORITY_LOW, 0), ^{
        NSData *data = [[errPipe fileHandleForReading] readDataToEndOfFile];
        if (data.length > 0) {
            NSString *output = [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
            NSLog(@"NHWatcher stderr: %@", output);
        }
    });
}

- (void)terminateViewer {
    if (_task && [_task isRunning]) {
        NSLog(@"NHWatcher: terminating viewer (PID %d)", [_task processIdentifier]);
        [_task terminate];
        [_task waitUntilExit];
    }
    _task = nil;
}

- (void)dealloc {
    [self terminateViewer];
}

@end
