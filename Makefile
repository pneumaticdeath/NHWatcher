SAVER_NAME = NHWatcher
SAVER_DIR = build/$(SAVER_NAME).saver
SAVER_CONTENTS = $(SAVER_DIR)/Contents
INSTALL_DIR = $(HOME)/Library/Screen\ Savers

.PHONY: all app saver install uninstall clean

all: saver

# Build the standalone Go app
app:
	go build -o build/nhwatcher ./cmd/nhwatcher/

# Build the .saver bundle
saver: app
	@mkdir -p $(SAVER_CONTENTS)/MacOS
	@mkdir -p $(SAVER_CONTENTS)/Resources
	@# Compile the ObjC screensaver wrapper as a Mach-O bundle
	clang -bundle \
		-framework ScreenSaver \
		-framework Cocoa \
		-fobjc-arc \
		-o $(SAVER_CONTENTS)/MacOS/$(SAVER_NAME) \
		screensaver/NHWatcherView.m
	@# Copy Info.plist
	cp screensaver/Info.plist $(SAVER_CONTENTS)/Info.plist
	@# Embed the Go binary and icon in Resources
	cp build/nhwatcher $(SAVER_CONTENTS)/Resources/nhwatcher
	cp icon.png $(SAVER_CONTENTS)/Resources/icon.png
	@echo "Built $(SAVER_DIR)"

# Install to ~/Library/Screen Savers
install: saver
	@# Kill any running screensaver engine so the new version loads
	-killall legacyScreenSaver 2>/dev/null || true
	@mkdir -p $(INSTALL_DIR)
	cp -R $(SAVER_DIR) $(INSTALL_DIR)/
	@echo "Installed to $(INSTALL_DIR)/$(SAVER_NAME).saver"
	@echo "Open System Settings > Screen Saver to select NH Watcher"

# Remove from ~/Library/Screen Savers
uninstall:
	-killall legacyScreenSaver 2>/dev/null || true
	rm -rf $(INSTALL_DIR)/$(SAVER_NAME).saver
	@echo "Uninstalled $(SAVER_NAME).saver"

# Test the screensaver directly (without installing)
test-saver: saver
	-killall legacyScreenSaver 2>/dev/null || true
	open $(SAVER_DIR)

clean:
	rm -rf build/
	rm -f nhwatcher

# Run the standalone app (no screensaver wrapper)
run: app
	./build/nhwatcher
