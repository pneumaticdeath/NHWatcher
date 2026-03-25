#import "NHWatcherView.h"

@implementation NHWatcherView {
    NSTask *_task;
    BOOL _isPreview;
}

- (instancetype)initWithFrame:(NSRect)frame isPreview:(BOOL)isPreview {
    self = [super initWithFrame:frame isPreview:isPreview];
    if (self) {
        _isPreview = isPreview;
        [self setAnimationTimeInterval:1.0/30.0];
    }
    return self;
}

- (void)startAnimation {
    [super startAnimation];

    // Don't launch the app in the small preview window
    if (_isPreview) {
        return;
    }

    [self launchViewer];
}

- (void)stopAnimation {
    [super stopAnimation];
    [self terminateViewer];
}

- (void)drawRect:(NSRect)rect {
    // Fill with black — the Go app renders in its own window
    [[NSColor blackColor] setFill];
    NSRectFill(rect);

    if (_isPreview) {
        // Show the icon in the preview pane
        NSBundle *bundle = [NSBundle bundleForClass:[self class]];
        NSString *iconPath = [bundle pathForResource:@"icon" ofType:@"png"];
        if (iconPath) {
            NSImage *icon = [[NSImage alloc] initWithContentsOfFile:iconPath];
            if (icon) {
                // Scale icon to fit preview with padding
                CGFloat padding = rect.size.width * 0.15;
                CGFloat side = fmin(rect.size.width, rect.size.height) - padding * 2;
                NSRect iconRect = NSMakeRect(
                    (rect.size.width - side) / 2,
                    (rect.size.height - side) / 2,
                    side, side
                );
                [icon drawInRect:iconRect
                        fromRect:NSZeroRect
                       operation:NSCompositingOperationSourceOver
                        fraction:0.8];
            }
        }
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

    // The nhwatcher binary is bundled inside Resources/
    NSBundle *bundle = [NSBundle bundleForClass:[self class]];
    NSString *binPath = [bundle pathForResource:@"nhwatcher" ofType:nil];

    if (!binPath) {
        NSLog(@"NHWatcher: nhwatcher binary not found in bundle Resources");
        return;
    }

    _task = [[NSTask alloc] init];
    [_task setExecutableURL:[NSURL fileURLWithPath:binPath]];
    [_task setArguments:@[@"--screensaver"]];

    // Capture stderr for logging
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

    // Log stderr output in the background
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
