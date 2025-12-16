package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/mxguardian/pst-import-tool/internal/imap"
	"github.com/mxguardian/pst-import-tool/internal/pst"
	"github.com/mxguardian/pst-import-tool/internal/state"
)

// Options holds the CLI configuration options
type Options struct {
	PSTFile     string
	Username    string
	Password    string
	Fresh       bool
	SkipDeleted bool
	SkipSent    bool
}

// Run executes the CLI import process
func Run(opts Options) {
	pstFile := opts.PSTFile
	username := opts.Username
	password := opts.Password
	fresh := opts.Fresh
	fmt.Println("MXGuardian PST Import")
	fmt.Println("=====================")

	// Initialize state management
	importState, err := state.NewImportState(pstFile, username)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize state: %v\n", err)
		os.Exit(1)
	}

	if !fresh {
		if err := importState.Load(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to load state: %v\n", err)
		}
	} else {
		importState.Clear()
	}

	if importState.HasExistingProgress() {
		uploaded, total := importState.GetProgress()
		fmt.Printf("Resuming: %d/%d messages already uploaded\n", uploaded, total)
		fmt.Printf("State file: %s\n", importState.StatePath())
		fmt.Println("(Use -fresh to start over)")
	}

	// Test IMAP connection
	fmt.Println("\nConnecting to IMAP server...")
	if err := imap.TestConnection(username, password); err != nil {
		fmt.Fprintf(os.Stderr, "IMAP connection failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Connected successfully")

	// Open PST file
	fmt.Printf("\nOpening PST file: %s\n", pstFile)
	extractor, err := pst.NewExtractor()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize: %v\n", err)
		os.Exit(1)
	}
	defer extractor.Close()

	if err := extractor.Open(pstFile); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open PST: %v\n", err)
		os.Exit(1)
	}

	// Connect to IMAP for uploading
	uploader, err := imap.NewUploader(username, password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect: %v\n", err)
		os.Exit(1)
	}
	defer uploader.Close()

	// Stream messages
	fmt.Println("\nStreaming messages...")

	var (
		currentFolder   string
		folderUploaded  int
		folderSkipped   int
		totalUploaded   int
		totalSkipped    int
		totalErrors     int
		saveCounter     int
		completedFolder = make(map[string]bool)
	)

	err = extractor.Process(
		// On folder start
		func(folderName string) (skip bool, err error) {
			// Print previous folder summary
			if currentFolder != "" && (folderUploaded > 0 || folderSkipped > 0) {
				if folderSkipped > 0 {
					fmt.Printf("  → %d uploaded, %d skipped\n", folderUploaded, folderSkipped)
				} else {
					fmt.Printf("  → %d uploaded\n", folderUploaded)
				}
				completedFolder[currentFolder] = true
			}

			currentFolder = folderName
			folderUploaded = 0
			folderSkipped = 0

			// Check if folder should be skipped based on options
			lowerFolder := strings.ToLower(folderName)
			if opts.SkipDeleted && (strings.Contains(lowerFolder, "deleted items") || strings.Contains(lowerFolder, "trash")) {
				fmt.Printf("[%s] skipping (--skip-deleted)\n", folderName)
				return true, nil
			}
			if opts.SkipSent && (strings.Contains(lowerFolder, "sent items") || lowerFolder == "sent") {
				fmt.Printf("[%s] skipping (--skip-sent)\n", folderName)
				return true, nil
			}

			// Check if folder already complete
			if importState.IsFolderComplete(folderName) {
				fmt.Printf("[%s] skipping (already complete)\n", folderName)
				return true, nil
			}

			fmt.Printf("[%s] ", folderName)
			return false, nil
		},
		// On each message
		func(folderName string, msg *pst.Message) error {
			// Check if already uploaded
			if importState.IsUploaded(msg.ID) {
				folderSkipped++
				totalSkipped++
				return nil
			}

			// Upload immediately
			if err := uploader.Upload(folderName, msg); err != nil {
				totalErrors++
				fmt.Printf("E")
				return nil
			}

			// Mark as uploaded
			importState.MarkUploaded(msg.ID)
			folderUploaded++
			totalUploaded++

			// Progress indicator
			if totalUploaded%50 == 0 {
				fmt.Printf(".")
			}

			// Save state periodically
			saveCounter++
			if saveCounter%50 == 0 {
				importState.Save()
			}

			return nil
		},
		nil,
	)

	// Print final folder summary
	if currentFolder != "" && (folderUploaded > 0 || folderSkipped > 0) {
		if folderSkipped > 0 {
			fmt.Printf("  → %d uploaded, %d skipped\n", folderUploaded, folderSkipped)
		} else {
			fmt.Printf("  → %d uploaded\n", folderUploaded)
		}
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "\nError during import: %v\n", err)
	}

	// Mark completed folders
	for folder := range completedFolder {
		importState.MarkFolderComplete(folder)
	}
	importState.Save()

	// Summary
	fmt.Println("\n=====================")
	fmt.Printf("Complete: %d uploaded", totalUploaded)
	if totalSkipped > 0 {
		fmt.Printf(", %d skipped", totalSkipped)
	}
	if totalErrors > 0 {
		fmt.Printf(", %d errors", totalErrors)
	}
	fmt.Println()

	if totalErrors == 0 {
		importState.Clear()
		fmt.Println("State cleaned up")
	} else {
		fmt.Printf("State saved to: %s\n", importState.StatePath())
		fmt.Println("Run again to retry")
	}
}
