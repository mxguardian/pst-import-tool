package gui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"

	"github.com/mxguardian/pst-import-tool/internal/imap"
	"github.com/mxguardian/pst-import-tool/internal/pst"
)

// App represents the main GUI application
type App struct {
	fyneApp    fyne.App
	mainWindow fyne.Window

	// UI elements
	pstPathLabel  *widget.Label
	pstSelectBtn  *widget.Button
	usernameEntry *widget.Entry
	passwordEntry *widget.Entry
	startBtn      *widget.Button
	cancelBtn     *widget.Button
	progressBar   *widget.ProgressBar
	statusLabel   *widget.Label
	logText       *widget.Entry

	// State
	pstPath   string
	importing bool
	cancel    chan struct{}
}

// NewApp creates a new GUI application
func NewApp() *App {
	return &App{
		cancel: make(chan struct{}),
	}
}

// Run starts the GUI application
func (a *App) Run() {
	a.fyneApp = app.New()
	a.mainWindow = a.fyneApp.NewWindow("MXGuardian PST Import")

	a.buildUI()

	a.mainWindow.Resize(fyne.NewSize(500, 400))
	a.mainWindow.ShowAndRun()
}

func (a *App) buildUI() {
	// PST file selection
	a.pstPathLabel = widget.NewLabel("No file selected")
	a.pstPathLabel.Wrapping = fyne.TextWrapWord

	a.pstSelectBtn = widget.NewButton("Select PST File...", a.selectPSTFile)

	pstBox := container.NewVBox(
		widget.NewLabel("PST File:"),
		a.pstPathLabel,
		a.pstSelectBtn,
	)

	// IMAP credentials
	a.usernameEntry = widget.NewEntry()
	a.usernameEntry.SetPlaceHolder("email@example.com")

	a.passwordEntry = widget.NewPasswordEntry()
	a.passwordEntry.SetPlaceHolder("Password")

	credentialsForm := container.NewVBox(
		widget.NewLabel("IMAP Credentials (mail.mxguardian.net):"),
		widget.NewLabel("Username:"),
		a.usernameEntry,
		widget.NewLabel("Password:"),
		a.passwordEntry,
	)

	// Buttons
	a.startBtn = widget.NewButton("Start Import", a.startImport)
	a.startBtn.Importance = widget.HighImportance

	a.cancelBtn = widget.NewButton("Cancel", a.cancelImport)
	a.cancelBtn.Disable()

	buttonBox := container.NewHBox(a.startBtn, a.cancelBtn)

	// Progress
	a.progressBar = widget.NewProgressBar()
	a.statusLabel = widget.NewLabel("Ready")

	// Log output
	a.logText = widget.NewMultiLineEntry()
	a.logText.SetMinRowsVisible(6)

	progressBox := container.NewVBox(
		a.progressBar,
		a.statusLabel,
		widget.NewLabel("Log:"),
		a.logText,
	)

	// Main layout
	content := container.NewVBox(
		pstBox,
		widget.NewSeparator(),
		credentialsForm,
		widget.NewSeparator(),
		buttonBox,
		widget.NewSeparator(),
		progressBox,
	)

	a.mainWindow.SetContent(container.NewPadded(content))
}

func (a *App) selectPSTFile() {
	fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil {
			a.log("Error selecting file: " + err.Error())
			return
		}
		if reader == nil {
			return
		}
		reader.Close()

		a.pstPath = reader.URI().Path()
		a.pstPathLabel.SetText(a.pstPath)
		a.log("Selected: " + a.pstPath)
	}, a.mainWindow)

	fd.SetFilter(storage.NewExtensionFileFilter([]string{".pst", ".PST"}))
	fd.Show()
}

func (a *App) startImport() {
	// Validate inputs
	if a.pstPath == "" {
		dialog.ShowError(fmt.Errorf("please select a PST file"), a.mainWindow)
		return
	}
	if a.usernameEntry.Text == "" {
		dialog.ShowError(fmt.Errorf("please enter your username"), a.mainWindow)
		return
	}
	if a.passwordEntry.Text == "" {
		dialog.ShowError(fmt.Errorf("please enter your password"), a.mainWindow)
		return
	}

	a.importing = true
	a.cancel = make(chan struct{})
	a.setUIEnabled(false)

	go a.runImport()
}

func (a *App) cancelImport() {
	if a.importing {
		close(a.cancel)
		a.log("Cancelling import...")
	}
}

func (a *App) runImport() {
	defer func() {
		a.importing = false
		a.setUIEnabled(true)
	}()

	// Test IMAP connection first
	a.setStatus("Testing IMAP connection...")
	a.log("Connecting to mail.mxguardian.net...")

	if err := imap.TestConnection(a.usernameEntry.Text, a.passwordEntry.Text); err != nil {
		a.log("Connection failed: " + err.Error())
		a.showError("IMAP connection failed", err)
		return
	}
	a.log("IMAP connection successful")

	// Open PST file
	a.setStatus("Opening PST file...")
	a.log("Opening: " + a.pstPath)

	extractor, err := pst.NewExtractor()
	if err != nil {
		a.log("Extractor error: " + err.Error())
		a.showError("Failed to initialize PST extractor", err)
		return
	}
	defer extractor.Close()

	if err := extractor.Open(a.pstPath); err != nil {
		a.log("Failed to open PST: " + err.Error())
		a.showError("Failed to open PST file", err)
		return
	}

	// Connect to IMAP for upload
	a.setStatus("Connecting to IMAP...")

	uploader, err := imap.NewUploader(a.usernameEntry.Text, a.passwordEntry.Text)
	if err != nil {
		a.log("Upload connection failed: " + err.Error())
		a.showError("Failed to connect to IMAP server", err)
		return
	}
	defer uploader.Close()

	// Stream messages
	a.log("Streaming messages...")

	var (
		currentFolder  string
		totalUploaded  int
		totalErrors    int
		cancelled      bool
	)

	err = extractor.Process(
		// On folder
		func(folderName string) (skip bool, err error) {
			select {
			case <-a.cancel:
				cancelled = true
				return false, fmt.Errorf("cancelled")
			default:
			}

			if currentFolder != folderName {
				currentFolder = folderName
				a.log(fmt.Sprintf("Processing: %s", folderName))
			}
			return false, nil
		},
		// On message
		func(folderName string, msg *pst.Message) error {
			select {
			case <-a.cancel:
				cancelled = true
				return fmt.Errorf("cancelled")
			default:
			}

			if err := uploader.Upload(folderName, msg); err != nil {
				totalErrors++
				return nil
			}

			totalUploaded++
			a.setStatus(fmt.Sprintf("Uploaded %d messages...", totalUploaded))

			return nil
		},
		nil,
	)

	if cancelled {
		a.setStatus("Import cancelled")
		a.log(fmt.Sprintf("Cancelled after %d messages", totalUploaded))
		return
	}

	if err != nil && !cancelled {
		a.log("Error: " + err.Error())
	}

	a.setStatus("Import complete!")
	a.setProgress(1.0)
	a.log(fmt.Sprintf("Completed: %d messages uploaded, %d errors", totalUploaded, totalErrors))

	fyne.Do(func() {
		dialog.ShowInformation("Success",
			fmt.Sprintf("PST import completed!\n%d messages uploaded", totalUploaded),
			a.mainWindow)
	})
}

func (a *App) setUIEnabled(enabled bool) {
	fyne.Do(func() {
		if enabled {
			a.pstSelectBtn.Enable()
			a.usernameEntry.Enable()
			a.passwordEntry.Enable()
			a.startBtn.Enable()
			a.cancelBtn.Disable()
		} else {
			a.pstSelectBtn.Disable()
			a.usernameEntry.Disable()
			a.passwordEntry.Disable()
			a.startBtn.Disable()
			a.cancelBtn.Enable()
		}
	})
}

func (a *App) setStatus(status string) {
	fyne.Do(func() {
		a.statusLabel.SetText(status)
	})
}

func (a *App) setProgress(progress float64) {
	fyne.Do(func() {
		a.progressBar.SetValue(progress)
	})
}

func (a *App) log(message string) {
	fyne.Do(func() {
		current := a.logText.Text
		if current != "" {
			current += "\n"
		}
		a.logText.SetText(current + message)
	})
}

func (a *App) showError(title string, err error) {
	fyne.Do(func() {
		dialog.ShowError(fmt.Errorf("%s: %v", title, err), a.mainWindow)
	})
}
