.PHONY: all clean macos-arm64 macos-amd64 windows-amd64 linux-amd64 cli

# Default: build for current platform with GUI
all:
	go build -o pst-import ./cmd/pst-import

# macOS ARM64 (GUI + CLI)
macos-arm64:
	mkdir -p dist
	GOOS=darwin GOARCH=arm64 go build -o dist/pst-import-macos-arm64 ./cmd/pst-import

# macOS AMD64 (GUI + CLI)
macos-amd64:
	mkdir -p dist
	GOOS=darwin GOARCH=amd64 go build -o dist/pst-import-macos-amd64 ./cmd/pst-import

# Windows AMD64 (CLI only - no CGO)
windows-amd64:
	mkdir -p dist
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o dist/pst-import.exe ./cmd/pst-import-cli

# Linux AMD64 (CLI only - no CGO)
linux-amd64:
	mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o dist/pst-import-linux-amd64 ./cmd/pst-import-cli

# Build all platforms
dist: macos-arm64 windows-amd64 linux-amd64

clean:
	rm -rf dist/
	rm -f pst-import
