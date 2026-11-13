# netbird-kindle: build and package. Needs Go, shellcheck, zip and shasum.
#   make test    vet and test nblink, lint the shell scripts
#   make dist    dist/netbird-kindle/ and dist/netbird-kindle.zip, ready to copy to a Kindle

BUILD   := -trimpath -ldflags "-s -w"
SCRIPTS := kindle/nbk kindle/install.sh kindle/uninstall.sh kindle/NetBird.sh
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo unknown)

.PHONY: test dist clean

test:
	cd link && go vet ./... && go test -race ./...
	shellcheck -s sh -S warning $(SCRIPTS)

dist: test
	rm -rf dist && mkdir -p dist/netbird-kindle/koreader-patch
	cd link && CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=6 go build $(BUILD) -o ../dist/netbird-kindle/nblink-arm .
	cd link && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(BUILD) -o ../dist/netbird-kindle/nblink-arm64 .
	cp kindle/nbk kindle/install.sh kindle/uninstall.sh kindle/NetBird.sh kindle/versions \
	   kindle/netbird-kindle.conf.example LICENSE README.md dist/netbird-kindle/
	cp kindle/koreader-patch/2-netbird-kindle.lua dist/netbird-kindle/koreader-patch/
	echo "$(VERSION)" > dist/netbird-kindle/VERSION
	cd dist && zip -qr netbird-kindle.zip netbird-kindle
	cd dist && shasum -a 256 netbird-kindle.zip netbird-kindle/nblink-arm netbird-kindle/nblink-arm64 > SHA256SUMS

clean:
	rm -rf dist
