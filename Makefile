SAVER_NAME = NHWatcher
VERSION = $(shell grep '^Version' FyneApp.toml | sed 's/.*"\(.*\)"/\1/')
SAVER_DIR = build/$(SAVER_NAME).saver
SAVER_CONTENTS = $(SAVER_DIR)/Contents
INSTALL_DIR = $(HOME)/Library/Screen\ Savers
RELEASE_ZIP = build/$(SAVER_NAME)-$(VERSION).saver.zip

# Code signing identity — set via environment or edit here
# Use: export DEVELOPER_ID="Developer ID Application: Mitch Patenaude (TEAMID)"
DEVELOPER_ID ?=

# Notarization — uses a keychain profile by default (simplest for local use).
# Create the profile once with:
#   xcrun notarytool store-credentials "notarytool-profile" \
#       --apple-id you@email.com --team-id TEAMID --password <app-specific-password>
#
# For CI, set NOTARIZE_APPLE_ID, NOTARIZE_TEAM_ID, and NOTARIZE_PASSWORD instead.
NOTARIZE_PROFILE ?= notarytool-profile
NOTARIZE_APPLE_ID ?=
NOTARIZE_TEAM_ID ?=
NOTARIZE_PASSWORD ?=

.PHONY: all app app-universal saver saver-universal install uninstall clean \
        sign sign-only notarize notarize-only release test-saver run

all: saver

# Build the standalone Go app (native arch)
app:
	cp screensaver/Resources/bundled.ttyrec internal/nao/bundled.ttyrec
	go build -o build/nhwatcher ./cmd/nhwatcher/

# Build universal (arm64 + amd64) Go binary
app-universal:
	cp screensaver/Resources/bundled.ttyrec internal/nao/bundled.ttyrec
	CGO_ENABLED=1 GOARCH=arm64 go build -o build/nhwatcher-arm64 ./cmd/nhwatcher/
	CGO_ENABLED=1 GOARCH=amd64 go build -o build/nhwatcher-amd64 ./cmd/nhwatcher/
	lipo -create -output build/nhwatcher build/nhwatcher-arm64 build/nhwatcher-amd64
	rm build/nhwatcher-arm64 build/nhwatcher-amd64

# Build the .saver bundle (native arch)
saver: app
	$(MAKE) _bundle

# Build the .saver bundle with universal binary
saver-universal: app-universal
	$(MAKE) _bundle

# Internal: assemble the .saver bundle (expects build/nhwatcher to exist)
_bundle:
	@mkdir -p $(SAVER_CONTENTS)/MacOS
	@mkdir -p $(SAVER_CONTENTS)/Resources
	clang -bundle \
		-framework ScreenSaver \
		-framework Cocoa \
		-fobjc-arc \
		-arch arm64 -arch x86_64 \
		-o $(SAVER_CONTENTS)/MacOS/$(SAVER_NAME) \
		screensaver/NHWatcherView.m
	cp screensaver/Info.plist $(SAVER_CONTENTS)/Info.plist
	cp build/nhwatcher $(SAVER_CONTENTS)/Resources/nhwatcher
	cp icon.png $(SAVER_CONTENTS)/Resources/icon.png
	cp preview.png $(SAVER_CONTENTS)/Resources/preview.png
	cp screensaver/Resources/bundled.ttyrec $(SAVER_CONTENTS)/Resources/bundled.ttyrec
	tiffutil -cathidpicheck thumbnail.png thumbnail@2x.png -out $(SAVER_CONTENTS)/Resources/thumbnail.tiff
	@echo "Built $(SAVER_DIR)"

# Sign the .saver bundle and its embedded binary.
# When called from CI (where build steps are separate), invoke without
# the saver-universal prerequisite: make saver-universal && make sign-only.
sign: saver-universal
	$(MAKE) sign-only

sign-only:
ifndef DEVELOPER_ID
	$(error DEVELOPER_ID is not set. Export it or pass it to make.)
endif
	codesign --force --options runtime --sign "$(DEVELOPER_ID)" \
		$(SAVER_CONTENTS)/Resources/nhwatcher
	codesign --force --options runtime --sign "$(DEVELOPER_ID)" \
		$(SAVER_DIR)
	codesign --verify --verbose $(SAVER_DIR)
	@echo "Signed $(SAVER_DIR)"

# Notarize the signed .saver bundle.
# notarize-only skips the rebuild; use from CI after sign-only.
notarize: sign
	$(MAKE) notarize-only

notarize-only:
	cd build && zip -r $(SAVER_NAME)-$(VERSION).saver.zip $(SAVER_NAME).saver
ifdef NOTARIZE_APPLE_ID
	xcrun notarytool submit $(RELEASE_ZIP) \
		--apple-id "$(NOTARIZE_APPLE_ID)" \
		--team-id "$(NOTARIZE_TEAM_ID)" \
		--password "$(NOTARIZE_PASSWORD)" \
		--wait
else
	xcrun notarytool submit $(RELEASE_ZIP) \
		--keychain-profile "$(NOTARIZE_PROFILE)" \
		--wait
endif
	xcrun stapler staple $(SAVER_DIR)
	@# Re-zip with the stapled ticket
	cd build && rm -f $(SAVER_NAME)-$(VERSION).saver.zip && \
		zip -r $(SAVER_NAME)-$(VERSION).saver.zip $(SAVER_NAME).saver
	@echo "Notarized and packaged: $(RELEASE_ZIP)"

# Build, sign, notarize, and package for release
release: notarize
	@echo "Release artifact: $(RELEASE_ZIP)"
	@shasum -a 256 $(RELEASE_ZIP)

# Install to ~/Library/Screen Savers
install: saver
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
