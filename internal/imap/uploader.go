package imap

import (
	"crypto/tls"
	"fmt"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"

	"github.com/mxguardian/pst-import-tool/internal/pst"
)

const (
	IMAPServer = "mail.mxguardian.net"
	IMAPPort   = 993
)

// Uploader handles uploading messages to IMAP
type Uploader struct {
	client         *client.Client
	username       string
	createdFolders map[string]bool
}

// NewUploader creates a new IMAP uploader and connects to the server
func NewUploader(username, password string) (*Uploader, error) {
	addr := fmt.Sprintf("%s:%d", IMAPServer, IMAPPort)

	// Connect with TLS
	c, err := client.DialTLS(addr, &tls.Config{
		ServerName: IMAPServer,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to IMAP server: %w", err)
	}

	// Login
	if err := c.Login(username, password); err != nil {
		c.Logout()
		return nil, fmt.Errorf("IMAP login failed: %w", err)
	}

	return &Uploader{
		client:         c,
		username:       username,
		createdFolders: make(map[string]bool),
	}, nil
}

// Close disconnects from the IMAP server
func (u *Uploader) Close() error {
	if u.client != nil {
		return u.client.Logout()
	}
	return nil
}

// Upload uploads a single message to the appropriate IMAP folder
func (u *Uploader) Upload(folderName string, msg *pst.Message) error {
	imapFolder := mapToIMAPFolder(folderName)

	// Create IMAP folder if needed
	if !u.createdFolders[imapFolder] {
		if err := u.createFolder(imapFolder); err != nil {
			// Log but continue - folder might already exist
		}
		u.createdFolders[imapFolder] = true
	}

	// Use the message's date or default to now
	msgDate := msg.Date
	if msgDate.IsZero() || msgDate.Year() < 1990 {
		msgDate = time.Now()
	}

	// Append to IMAP folder
	literal := strings.NewReader(string(msg.Content))
	return u.client.Append(imapFolder, []string{imap.SeenFlag}, msgDate, literal)
}

// createFolder creates an IMAP folder if it doesn't exist
func (u *Uploader) createFolder(folder string) error {
	err := u.client.Create(folder)
	if err != nil {
		// Check if it's just because folder exists
		if strings.Contains(err.Error(), "ALREADYEXISTS") ||
			strings.Contains(err.Error(), "already exists") ||
			strings.Contains(err.Error(), "Mailbox exists") {
			return nil
		}
		return err
	}
	return nil
}

// mapToIMAPFolder converts a PST folder name to an IMAP folder name
// MXGuardian IMAP requires folders to be under INBOX namespace (INBOX.FolderName)
func mapToIMAPFolder(folderName string) string {
	// Clean up folder name
	folder := strings.TrimSpace(folderName)

	// Handle root folder
	if folder == "" {
		return "INBOX"
	}

	// Remove common PST root prefixes
	folder = strings.TrimPrefix(folder, "Top of Personal Folders/")
	folder = strings.TrimPrefix(folder, "Root - Mailbox/")
	folder = strings.TrimPrefix(folder, "Root/")

	// Convert path separators to IMAP hierarchy separator (.)
	folder = strings.ReplaceAll(folder, "/", ".")
	folder = strings.ReplaceAll(folder, "\\", ".")

	// Sanitize each component of the folder path
	parts := strings.Split(folder, ".")
	for i, part := range parts {
		parts[i] = sanitizeFolderName(part)
	}

	// Remove empty parts
	cleanParts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			cleanParts = append(cleanParts, part)
		}
	}
	folder = strings.Join(cleanParts, ".")

	// Map common PST folder names to IMAP
	lowerFolder := strings.ToLower(folder)

	// Check for exact matches first
	switch lowerFolder {
	case "inbox":
		return "INBOX"
	case "sent items", "sent":
		return "INBOX.Sent"
	case "deleted items", "trash":
		return "INBOX.Trash"
	case "drafts":
		return "INBOX.Drafts"
	case "junk e-mail", "junk", "spam":
		return "INBOX.Junk"
	}

	// Handle subfolders
	switch {
	case strings.HasPrefix(lowerFolder, "inbox."):
		return "INBOX." + folder[6:]
	case strings.HasPrefix(lowerFolder, "sent items."):
		return "INBOX.Sent." + folder[11:]
	case strings.HasPrefix(lowerFolder, "deleted items."):
		return "INBOX.Trash." + folder[14:]
	}

	// For any other folder, put it under INBOX
	return "INBOX." + folder
}

// sanitizeFolderName removes or replaces characters that are invalid in IMAP folder names
func sanitizeFolderName(name string) string {
	name = strings.TrimSpace(name)

	replacer := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		"*", "_",
		"%", "_",
		"?", "_",
		"\"", "'",
		"<", "(",
		">", ")",
		"|", "-",
		":", "-",
		"\x00", "",
		"\r", "",
		"\n", "",
		"\t", " ",
	)
	name = replacer.Replace(name)

	var result strings.Builder
	for _, r := range name {
		if r >= 32 && r != 127 {
			result.WriteRune(r)
		}
	}
	name = result.String()

	if name == "" {
		name = "Unnamed"
	}

	if len(name) > 200 {
		name = name[:200]
	}

	return name
}

// TestConnection tests the IMAP connection without uploading
func TestConnection(username, password string) error {
	uploader, err := NewUploader(username, password)
	if err != nil {
		return err
	}
	defer uploader.Close()
	return nil
}
