# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

MXGuardian PST Import Utility - Imports Microsoft Outlook PST files into MXGuardian IMAP mailboxes.

- IMAP server: `mail.mxguardian.net:993` (hardcoded)
- User provides: PST file, IMAP username, IMAP password
- Pure Go implementation (no external dependencies like readpst)

## Build Commands

```bash
# Build for current platform
make

# Build all distributions
make dist

# Clean build artifacts
make clean
```

## Architecture

```
cmd/
  pst-import/         - GUI + CLI entry point (macOS only due to CGO)
  pst-import-cli/     - CLI-only entry point (cross-platform, no CGO)

internal/
  cli/run.go          - Shared CLI logic
  gui/app.go          - Fyne GUI (macOS only)
  pst/extractor.go    - PST reading via go-pst library (streaming)
  imap/uploader.go    - IMAP client, uploads messages via APPEND
  state/state.go      - Resume state management (bloom filter)
```

## Data Flow

PST → go-pst (streams message-by-message) → IMAP APPEND

Messages are processed one at a time to minimize memory usage.

## Key Dependencies

- `github.com/mooijtech/go-pst/v6` - Pure Go PST parsing
- `github.com/emersion/go-imap` - IMAP client
- `fyne.io/fyne/v2` - GUI framework (macOS builds only)

## Platform Support

- macOS: GUI + CLI (requires CGO for Fyne)
- Windows/Linux: CLI only (no CGO, cross-compilable)

## Naming

There is no space in "MXGuardian".
