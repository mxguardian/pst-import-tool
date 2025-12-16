.PHONY: all clean macos-arm64 macos-amd64 windows-amd64 linux-amd64 cli

# Default: build for current platform with GUI
all:
	go build -o pst-import ./cmd/pst-import

# macOS ARM64 (GUI + CLI)
macos-arm64:
	mkdir -p dist/macos-arm64
	GOOS=darwin GOARCH=arm64 go build -o dist/macos-arm64/pst-import ./cmd/pst-import

# macOS AMD64 (GUI + CLI)
macos-amd64:
	mkdir -p dist/macos-amd64
	GOOS=darwin GOARCH=amd64 go build -o dist/macos-amd64/pst-import ./cmd/pst-import

# Windows AMD64 (CLI only - no CGO)
windows-amd64:
	mkdir -p dist/windows-amd64
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o dist/windows-amd64/pst-import.exe ./cmd/pst-import-cli

# Linux AMD64 (CLI only - no CGO)
linux-amd64:
	mkdir -p dist/linux-amd64
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o dist/linux-amd64/pst-import ./cmd/pst-import-cli

# Build all platforms
dist: macos-arm64 windows-amd64 linux-amd64

clean:
	rm -rf dist/
	rm -f pst-import
