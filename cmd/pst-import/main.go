package main

import (
	"flag"

	"github.com/mxguardian/pst-import-tool/internal/cli"
	"github.com/mxguardian/pst-import-tool/internal/gui"
)

func main() {
	pstFile := flag.String("pst", "", "Path to PST file")
	username := flag.String("user", "", "IMAP username")
	password := flag.String("pass", "", "IMAP password")
	fresh := flag.Bool("fresh", false, "Start fresh, ignoring any saved progress")
	skipDeleted := flag.Bool("skip-deleted", false, "Skip Deleted Items folder")
	skipSent := flag.Bool("skip-sent", false, "Skip Sent Items folder")
	flag.Parse()

	// If CLI args provided, run in CLI mode
	if *pstFile != "" && *username != "" && *password != "" {
		cli.Run(cli.Options{
			PSTFile:     *pstFile,
			Username:    *username,
			Password:    *password,
			Fresh:       *fresh,
			SkipDeleted: *skipDeleted,
			SkipSent:    *skipSent,
		})
		return
	}

	// Otherwise run GUI
	app := gui.NewApp()
	app.Run()
}
