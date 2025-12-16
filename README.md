# MXGuardian PST Import Tool

Import Microsoft Outlook PST files into your MXGuardian mailbox.

## Download

Download the appropriate version for your platform from the [Releases](https://github.com/mxguardian/pst-import-tool/releases) page:

| Platform | File |
|----------|------|
| macOS (Apple Silicon) | `pst-import-macos-arm64` |
| Windows | `pst-import.exe` |
| Linux | `pst-import-linux-amd64` |

## Usage

```bash
pst-import --pst <file> --user <username> --pass <password> [options]
```

### Required Arguments

| Argument | Description |
|----------|-------------|
| `--pst` | Path to the PST file |
| `--user` | Your MXGuardian email address |
| `--pass` | Your MXGuardian password |

### Optional Arguments

| Argument | Description |
|----------|-------------|
| `--skip-deleted` | Skip importing Deleted Items folder |
| `--skip-sent` | Skip importing Sent Items folder |
| `--fresh` | Start over, ignoring any saved progress |

### Examples

Import all folders:
```bash
pst-import --pst archive.pst --user you@example.com --pass yourpassword
```

Skip deleted and sent items:
```bash
pst-import --pst archive.pst --user you@example.com --pass yourpassword --skip-deleted --skip-sent
```

## Resume Support

If the import is interrupted, simply run the same command again. The tool automatically tracks progress and resumes where it left off.

To start over from the beginning, add the `--fresh` flag.

## Platform Notes

- **macOS**: Includes both GUI and command-line interface. Double-click to launch GUI, or run from terminal for CLI.
- **Windows/Linux**: Command-line only.

## Troubleshooting

### Connection Failed
- Verify your MXGuardian username and password
- Check your internet connection
- Ensure mail.mxguardian.net is accessible on port 993

### Import Errors
- The tool will retry failed messages on the next run
- Check that the PST file is not corrupted
- Large PST files (>10GB) may take several hours

## Support

For issues or questions, contact [MXGuardian Support](https://www.mxguardian.net/support/).

## Disclaimer

THIS SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED. USE AT YOUR OWN RISK. Always maintain backups of your PST files before importing. MXGuardian is not responsible for any data loss or corruption that may occur during the import process.

## License

Copyright 2025 MXGuardian LLC

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) for details.
