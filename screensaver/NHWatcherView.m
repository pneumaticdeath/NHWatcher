#import "NHWatcherView.h"
#import <ScreenSaver/ScreenSaver.h>
#import <signal.h>

static NSString * const kBundleID    = @"io.patenaude.NHWatcher";
static NSString * const kServerNAO   = @"serverNAO";
static NSString * const kServerHdfUS = @"serverHdfUS";
static NSString * const kServerHdfEU = @"serverHdfEU";
static NSString * const kServerHdfAU = @"serverHdfAU";
static NSString * const kOverscan    = @"overscanPercent";

@implementation NHWatcherView {
    NSTask *_viewerTask;
    NSPipe *_framePipe;
    NSImage *_latestFrame;
    NSMutableData *_readBuffer;
    BOOL _isPreview;
    BOOL _reading;
    // Configure sheet
    NSWindow *_configSheet;
    NSButton *_chkNAO;
    NSButton *_chkHdfUS;
    NSButton *_chkHdfEU;
    NSButton *_chkHdfAU;
    NSSlider *_overscanSlider;
    NSTextField *_overscanLabel;
}

- (ScreenSaverDefaults *)defaults {
    return [ScreenSaverDefaults defaultsForModuleWithName:kBundleID];
}

- (instancetype)initWithFrame:(NSRect)frame isPreview:(BOOL)isPreview {
    self = [super initWithFrame:frame isPreview:isPreview];
    if (self) {
        _isPreview = isPreview;
        _readBuffer = [NSMutableData data];
        [self setAnimationTimeInterval:1.0/10.0];

        ScreenSaverDefaults *defaults = [self defaults];
        [defaults registerDefaults:@{
            kServerNAO:   @YES,
            kServerHdfUS: @YES,
            kServerHdfEU: @YES,
            kServerHdfAU: @YES,
            kOverscan:    @3.0,
        }];
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
        CGFloat pct = [[self defaults] floatForKey:kOverscan] / 100.0;
        CGFloat inset = fmin(rect.size.width, rect.size.height) * pct;
        NSRect safeRect = NSInsetRect(rect, inset, inset);
        [_latestFrame drawInRect:safeRect
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

- (BOOL)hasConfigureSheet { return YES; }

- (NSWindow *)configureSheet {
    if (_configSheet) return _configSheet;

    CGFloat w = 340, h = 260;
    _configSheet = [[NSWindow alloc]
        initWithContentRect:NSMakeRect(0, 0, w, h)
                  styleMask:NSWindowStyleMaskTitled
                    backing:NSBackingStoreBuffered
                      defer:NO];
    [_configSheet setTitle:@"NH Watcher Options"];

    NSView *content = [_configSheet contentView];
    CGFloat y = h - 30;
    CGFloat margin = 20;

    // --- Servers section ---
    NSTextField *serverTitle = [NSTextField labelWithString:@"Servers:"];
    [serverTitle setFont:[NSFont boldSystemFontOfSize:13]];
    [serverTitle setFrame:NSMakeRect(margin, y, w - margin * 2, 20)];
    [content addSubview:serverTitle];
    y -= 26;

    _chkNAO = [self checkboxWithTitle:@"nethack.alt.org (NAO)" frame:NSMakeRect(margin + 10, y, w - margin * 2, 20)];
    [content addSubview:_chkNAO];
    y -= 24;

    _chkHdfUS = [self checkboxWithTitle:@"us.hardfought.org" frame:NSMakeRect(margin + 10, y, w - margin * 2, 20)];
    [content addSubview:_chkHdfUS];
    y -= 24;

    _chkHdfEU = [self checkboxWithTitle:@"eu.hardfought.org" frame:NSMakeRect(margin + 10, y, w - margin * 2, 20)];
    [content addSubview:_chkHdfEU];
    y -= 24;

    _chkHdfAU = [self checkboxWithTitle:@"au.hardfought.org" frame:NSMakeRect(margin + 10, y, w - margin * 2, 20)];
    [content addSubview:_chkHdfAU];
    y -= 30;

    // --- Overscan section ---
    NSTextField *overscanTitle = [NSTextField labelWithString:@"Overscan margin:"];
    [overscanTitle setFont:[NSFont boldSystemFontOfSize:13]];
    [overscanTitle setFrame:NSMakeRect(margin, y, 140, 20)];
    [content addSubview:overscanTitle];

    _overscanLabel = [NSTextField labelWithString:@"3%"];
    [_overscanLabel setFrame:NSMakeRect(w - margin - 40, y, 40, 20)];
    [_overscanLabel setAlignment:NSTextAlignmentRight];
    [content addSubview:_overscanLabel];
    y -= 24;

    _overscanSlider = [[NSSlider alloc] initWithFrame:NSMakeRect(margin + 10, y, w - margin * 2 - 10, 20)];
    [_overscanSlider setMinValue:0.0];
    [_overscanSlider setMaxValue:10.0];
    [_overscanSlider setNumberOfTickMarks:11];
    [_overscanSlider setAllowsTickMarkValuesOnly:YES];
    [_overscanSlider setTarget:self];
    [_overscanSlider setAction:@selector(overscanChanged:)];
    [content addSubview:_overscanSlider];
    y -= 36;

    // --- Buttons ---
    NSButton *okBtn = [[NSButton alloc] initWithFrame:NSMakeRect(w - margin - 80, y, 80, 30)];
    [okBtn setTitle:@"OK"];
    [okBtn setBezelStyle:NSBezelStyleRounded];
    [okBtn setKeyEquivalent:@"\r"];
    [okBtn setTarget:self];
    [okBtn setAction:@selector(configOK:)];
    [content addSubview:okBtn];

    NSButton *cancelBtn = [[NSButton alloc] initWithFrame:NSMakeRect(w - margin - 170, y, 80, 30)];
    [cancelBtn setTitle:@"Cancel"];
    [cancelBtn setBezelStyle:NSBezelStyleRounded];
    [cancelBtn setKeyEquivalent:@"\033"];
    [cancelBtn setTarget:self];
    [cancelBtn setAction:@selector(configCancel:)];
    [content addSubview:cancelBtn];

    // Load current values
    [self loadConfigValues];

    return _configSheet;
}

- (NSButton *)checkboxWithTitle:(NSString *)title frame:(NSRect)frame {
    NSButton *btn = [[NSButton alloc] initWithFrame:frame];
    [btn setButtonType:NSButtonTypeSwitch];
    [btn setTitle:title];
    return btn;
}

- (void)loadConfigValues {
    ScreenSaverDefaults *defaults = [self defaults];
    [_chkNAO setState:[defaults boolForKey:kServerNAO] ? NSControlStateValueOn : NSControlStateValueOff];
    [_chkHdfUS setState:[defaults boolForKey:kServerHdfUS] ? NSControlStateValueOn : NSControlStateValueOff];
    [_chkHdfEU setState:[defaults boolForKey:kServerHdfEU] ? NSControlStateValueOn : NSControlStateValueOff];
    [_chkHdfAU setState:[defaults boolForKey:kServerHdfAU] ? NSControlStateValueOn : NSControlStateValueOff];
    CGFloat pct = [defaults floatForKey:kOverscan];
    [_overscanSlider setFloatValue:pct];
    [_overscanLabel setStringValue:[NSString stringWithFormat:@"%.0f%%", pct]];
}

- (void)overscanChanged:(id)sender {
    [_overscanLabel setStringValue:[NSString stringWithFormat:@"%.0f%%", [_overscanSlider floatValue]]];
}

- (void)configOK:(id)sender {
    ScreenSaverDefaults *defaults = [self defaults];
    [defaults setBool:([_chkNAO state] == NSControlStateValueOn) forKey:kServerNAO];
    [defaults setBool:([_chkHdfUS state] == NSControlStateValueOn) forKey:kServerHdfUS];
    [defaults setBool:([_chkHdfEU state] == NSControlStateValueOn) forKey:kServerHdfEU];
    [defaults setBool:([_chkHdfAU state] == NSControlStateValueOn) forKey:kServerHdfAU];
    [defaults setFloat:[_overscanSlider floatValue] forKey:kOverscan];
    [defaults synchronize];

    [self dismissConfigSheet];
}

- (void)configCancel:(id)sender {
    [self loadConfigValues]; // revert UI to saved values
    [self dismissConfigSheet];
}

- (void)dismissConfigSheet {
    if (_configSheet) {
        [_configSheet.sheetParent endSheet:_configSheet];
    }
}

- (void)launchViewer {
    if (_viewerTask && [_viewerTask isRunning]) return;

    NSBundle *bundle = [NSBundle bundleForClass:[self class]];
    NSString *binPath = [bundle pathForResource:@"nhwatcher" ofType:nil];
    if (!binPath) {
        NSLog(@"NHWatcher: nhwatcher binary not found in bundle Resources");
        return;
    }

    // Build --servers flag from preferences
    ScreenSaverDefaults *defaults = [self defaults];
    NSMutableArray *servers = [NSMutableArray array];
    if ([defaults boolForKey:kServerNAO])   [servers addObject:@"nao"];
    if ([defaults boolForKey:kServerHdfUS]) [servers addObject:@"hdf-us"];
    if ([defaults boolForKey:kServerHdfEU]) [servers addObject:@"hdf-eu"];
    if ([defaults boolForKey:kServerHdfAU]) [servers addObject:@"hdf-au"];
    // If nothing is selected, default to all
    if ([servers count] == 0) {
        [servers addObjectsFromArray:@[@"nao", @"hdf-us", @"hdf-eu", @"hdf-au"]];
    }
    NSString *serverArg = [servers componentsJoinedByString:@","];

    _framePipe = [NSPipe pipe];
    _viewerTask = [[NSTask alloc] init];
    [_viewerTask setExecutableURL:[NSURL fileURLWithPath:binPath]];
    [_viewerTask setArguments:@[@"--screensaver", @"--servers", serverArg]];
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
