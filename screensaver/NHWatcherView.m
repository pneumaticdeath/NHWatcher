#import "NHWatcherView.h"
#import <signal.h>

@implementation NHWatcherView {
    NSTask *_viewerTask;
    NSPipe *_framePipe;
    NSImage *_latestFrame;
    NSMutableData *_readBuffer;
    BOOL _isPreview;
    BOOL _reading;
}

- (instancetype)initWithFrame:(NSRect)frame isPreview:(BOOL)isPreview {
    self = [super initWithFrame:frame isPreview:isPreview];
    if (self) {
        _isPreview = isPreview;
        _readBuffer = [NSMutableData data];
        [self setAnimationTimeInterval:1.0/10.0];
    }
    return self;
}

- (void)startAnimation {
    [super startAnimation];
    if (_isPreview) return;
    [self launchViewer];
}

- (void)stopAnimation {
    [super stopAnimation];
    [self terminateViewer];
}

- (void)animateOneFrame {
    [self setNeedsDisplay:YES];
}

- (void)drawRect:(NSRect)rect {
    [[NSColor blackColor] setFill];
    NSRectFill(rect);

    if (_isPreview) {
        NSBundle *bundle = [NSBundle bundleForClass:[self class]];
        NSString *iconPath = [bundle pathForResource:@"icon" ofType:@"png"];
        if (iconPath) {
            NSImage *icon = [[NSImage alloc] initWithContentsOfFile:iconPath];
            if (icon) {
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
        return;
    }

    if (_latestFrame) {
        [_latestFrame drawInRect:rect
                        fromRect:NSZeroRect
                       operation:NSCompositingOperationSourceOver
                        fraction:1.0];
    }
}

// Read length-prefixed PNG frames from the viewer's stdout pipe.
// Frame format: [4-byte big-endian uint32 length][PNG data]
- (void)readFramesFromPipe:(NSFileHandle *)handle {
    _reading = YES;
    dispatch_async(dispatch_get_global_queue(DISPATCH_QUEUE_PRIORITY_DEFAULT, 0), ^{
        while (self->_reading) {
            @autoreleasepool {
                // Read 4-byte length header
                NSData *lengthData = [self readExactly:4 from:handle];
                if (!lengthData) break;

                const uint8_t *bytes = [lengthData bytes];
                uint32_t length = ((uint32_t)bytes[0] << 24) |
                                  ((uint32_t)bytes[1] << 16) |
                                  ((uint32_t)bytes[2] << 8)  |
                                  ((uint32_t)bytes[3]);

                if (length == 0 || length > 10 * 1024 * 1024) {
                    NSLog(@"NHWatcher: invalid frame length: %u", length);
                    break;
                }

                // Read PNG data
                NSData *pngData = [self readExactly:length from:handle];
                if (!pngData) break;

                NSImage *frame = [[NSImage alloc] initWithData:pngData];
                if (frame) {
                    self->_latestFrame = frame;
                }
            }
        }
        NSLog(@"NHWatcher: frame reader exited");
    });
}

- (NSData *)readExactly:(NSUInteger)count from:(NSFileHandle *)handle {
    NSMutableData *result = [NSMutableData dataWithCapacity:count];
    while ([result length] < count) {
        @try {
            NSData *chunk = [handle readDataOfLength:count - [result length]];
            if ([chunk length] == 0) return nil;  // EOF
            [result appendData:chunk];
        } @catch (NSException *e) {
            NSLog(@"NHWatcher: pipe read error: %@", e);
            return nil;
        }
    }
    return result;
}

- (BOOL)hasConfigureSheet { return NO; }
- (NSWindow *)configureSheet { return nil; }

- (void)launchViewer {
    if (_viewerTask && [_viewerTask isRunning]) return;

    NSBundle *bundle = [NSBundle bundleForClass:[self class]];
    NSString *binPath = [bundle pathForResource:@"nhwatcher" ofType:nil];
    if (!binPath) {
        NSLog(@"NHWatcher: nhwatcher binary not found in bundle Resources");
        return;
    }

    _framePipe = [NSPipe pipe];
    _viewerTask = [[NSTask alloc] init];
    [_viewerTask setExecutableURL:[NSURL fileURLWithPath:binPath]];
    [_viewerTask setArguments:@[@"--screensaver"]];
    [_viewerTask setStandardOutput:_framePipe];
    [_viewerTask setEnvironment:@{
        @"HOME": NSHomeDirectory(),
        @"USER": NSUserName(),
        @"NHWATCHER_SCREENSAVER": @"1"
    }];

    NSLog(@"NHWatcher: launching viewer at %@", binPath);

    NSError *error = nil;
    [_viewerTask launchAndReturnError:&error];
    if (error) {
        NSLog(@"NHWatcher: failed to launch: %@", error);
        return;
    }

    NSLog(@"NHWatcher: viewer launched (PID %d)", [_viewerTask processIdentifier]);
    [self readFramesFromPipe:[_framePipe fileHandleForReading]];
}

- (void)terminateViewer {
    _reading = NO;
    if (_viewerTask && [_viewerTask isRunning]) {
        NSLog(@"NHWatcher: terminating viewer (PID %d)", [_viewerTask processIdentifier]);
        [_viewerTask terminate];
        [_viewerTask waitUntilExit];
    }
    _viewerTask = nil;
    _framePipe = nil;
    _latestFrame = nil;
}

- (void)dealloc {
    [self terminateViewer];
}

@end
