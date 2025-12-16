// CLI-only version of PST Import tool (no GUI dependencies)
// This can be cross-compiled for Windows/Linux without CGO
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/mxguardian/pst-import-tool/internal/cli"
)

func main() {
	pstFile := flag.String("pst", "", "Path to PST file (required)")
	username := flag.String("user", "", "IMAP username (required)")
	password := flag.String("pass", "", "IMAP password (required)")
	fresh := flag.Bool("fresh", false, "Start fresh, ignoring any saved progress")
	skipDeleted := flag.Bool("skip-deleted", false, "Skip Deleted Items folder")
	skipSent := flag.Bool("skip-sent", false, "Skip Sent Items folder")
	flag.Parse()

	if *pstFile == "" || *username == "" || *password == "" {
		fmt.Println("MXGuardian PST Import Tool")
		fmt.Println()
		fmt.Println("Usage: pst-import --pst <file> --user <username> --pass <password> [options]")
		fmt.Println()
		fmt.Println("Required:")
		fmt.Println("  --pst <file>       Path to PST file")
		fmt.Println("  --user <username>  IMAP username (email address)")
		fmt.Println("  --pass <password>  IMAP password")
		fmt.Println()
		fmt.Println("Options:")
		fmt.Println("  --skip-deleted     Skip Deleted Items folder")
		fmt.Println("  --skip-sent        Skip Sent Items folder")
		fmt.Println("  --fresh            Start fresh, ignoring any saved progress")
		os.Exit(1)
	}

	cli.Run(cli.Options{
		PSTFile:     *pstFile,
		Username:    *username,
		Password:    *password,
		Fresh:       *fresh,
		SkipDeleted: *skipDeleted,
		SkipSent:    *skipSent,
	})
}
