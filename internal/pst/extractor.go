package pst

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"os"
	"strings"
	"time"

	"github.com/mooijtech/go-pst/v6/pkg"
	"github.com/mooijtech/go-pst/v6/pkg/properties"
)

// Message represents an email message ready for upload
type Message struct {
	ID      string    // Message-ID for tracking
	Date    time.Time // Original date for IMAP INTERNALDATE
	Content []byte    // RFC822 content
}

// MessageCallback is called for each message as it's read from the PST
// Return an error to stop processing
type MessageCallback func(folderName string, msg *Message) error

// FolderCallback is called when starting a new folder
// Returns (skip bool, err error) - set skip=true to skip this folder
type FolderCallback func(folderName string) (skip bool, err error)

// ProgressCallback is called with status updates
type ProgressCallback func(message string)

// Extractor handles PST file reading using pure Go
type Extractor struct {
	reader  io.ReadCloser
	pstFile *pst.File
}

// NewExtractor creates a new PST extractor
func NewExtractor() (*Extractor, error) {
	return &Extractor{}, nil
}

// Open opens a PST file for reading
func (e *Extractor) Open(pstPath string) error {
	reader, err := os.Open(pstPath)
	if err != nil {
		return fmt.Errorf("failed to open PST file: %w", err)
	}
	e.reader = reader

	pstFile, err := pst.New(reader)
	if err != nil {
		reader.Close()
		return fmt.Errorf("failed to parse PST file: %w", err)
	}
	e.pstFile = pstFile

	return nil
}

// Process streams through the PST file, calling callbacks for each folder and message
// Messages are processed one at a time - only one message is in memory at once
func (e *Extractor) Process(
	onFolder FolderCallback,
	onMessage MessageCallback,
	onProgress ProgressCallback,
) error {
	if e.pstFile == nil {
		return fmt.Errorf("PST file not opened")
	}

	return e.pstFile.WalkFolders(func(folder *pst.Folder) error {
		folderName := folder.Name

		if onProgress != nil {
			onProgress(fmt.Sprintf("Processing folder: %s", folderName))
		}

		// Check if we should skip this folder
		if onFolder != nil {
			skip, err := onFolder(folderName)
			if err != nil {
				return err
			}
			if skip {
				return nil
			}
		}

		// Get messages in this folder
		messageIterator, err := folder.GetMessageIterator()
		if err != nil {
			// Some folders might not have messages
			return nil
		}

		for messageIterator.Next() {
			msg := messageIterator.Value()

			// Get the properties - only process email messages
			msgProps, ok := msg.Properties.(*properties.Message)
			if !ok {
				continue
			}

			// Populate the properties from the PST
			if err := msg.PropertyContext.Populate(msgProps, msg.LocalDescriptors); err != nil {
				continue
			}

			// Build RFC822 message
			content, msgID, msgDate := buildRFC822Message(msgProps)
			if content == nil {
				continue
			}

			// Call the message callback immediately - message is uploaded here
			if onMessage != nil {
				pstMsg := &Message{
					ID:      msgID,
					Date:    msgDate,
					Content: content,
				}
				if err := onMessage(folderName, pstMsg); err != nil {
					return err
				}
			}
			// Message goes out of scope here - memory freed
		}

		return messageIterator.Err()
	})
}

// Close closes the PST file
func (e *Extractor) Close() error {
	if e.pstFile != nil {
		e.pstFile.Cleanup()
		e.pstFile = nil
	}
	if e.reader != nil {
		e.reader.Close()
		e.reader = nil
	}
	return nil
}

// buildRFC822Message constructs an RFC822 email from PST message properties
func buildRFC822Message(msg *properties.Message) ([]byte, string, time.Time) {
	var buf bytes.Buffer

	// Try to use original transport headers if available
	transportHeaders := msg.GetTransportMessageHeaders()

	// Get message metadata
	messageID := msg.GetInternetMessageId()
	if messageID == "" {
		// Generate a fallback Message-ID
		messageID = fmt.Sprintf("<%d.pst-import@localhost>", time.Now().UnixNano())
	}

	// Parse the date (GetClientSubmitTime returns Unix timestamp in seconds)
	msgDate := time.Now()
	if clientSubmitTime := msg.GetClientSubmitTime(); clientSubmitTime > 0 {
		parsed := time.Unix(clientSubmitTime, 0)
		// Validate the date is reasonable (between 1990 and 2100)
		if parsed.Year() >= 1990 && parsed.Year() <= 2100 {
			msgDate = parsed
		}
	}
	// Also try message delivery time
	if msgDate.Year() < 1990 {
		if deliveryTime := msg.GetMessageDeliveryTime(); deliveryTime > 0 {
			parsed := time.Unix(deliveryTime, 0)
			if parsed.Year() >= 1990 && parsed.Year() <= 2100 {
				msgDate = parsed
			}
		}
	}

	// Get body content
	bodyText := msg.GetBody()
	bodyHTML := msg.GetBodyHtml()

	if transportHeaders != "" {
		// Use original headers, but we may need to add body
		buf.WriteString(transportHeaders)

		// Ensure headers end with blank line
		if !strings.HasSuffix(transportHeaders, "\r\n\r\n") && !strings.HasSuffix(transportHeaders, "\n\n") {
			if !strings.HasSuffix(transportHeaders, "\r\n") && !strings.HasSuffix(transportHeaders, "\n") {
				buf.WriteString("\r\n")
			}
			buf.WriteString("\r\n")
		}

		// Check if headers indicate multipart
		if strings.Contains(strings.ToLower(transportHeaders), "multipart") {
			// Original message was multipart, body should follow headers
			buf.WriteString(bodyText)
		} else {
			// Simple message, append body
			if bodyText != "" {
				buf.WriteString(bodyText)
			} else if bodyHTML != "" {
				buf.WriteString(bodyHTML)
			}
		}
	} else {
		// Build headers from scratch
		writeHeader(&buf, "Message-ID", messageID)
		writeHeader(&buf, "Date", msgDate.Format(time.RFC1123Z))
		writeHeader(&buf, "Subject", encodeHeader(msg.GetSubject()))
		writeHeader(&buf, "From", formatAddress(msg.GetSenderName(), msg.GetSenderEmailAddress()))
		writeHeader(&buf, "To", msg.GetDisplayTo())

		if cc := msg.GetDisplayCc(); cc != "" {
			writeHeader(&buf, "Cc", cc)
		}

		if inReplyTo := msg.GetInReplyToId(); inReplyTo != "" {
			writeHeader(&buf, "In-Reply-To", inReplyTo)
		}

		if refs := msg.GetInternetReferences(); refs != "" {
			writeHeader(&buf, "References", refs)
		}

		// Determine content type and write body
		if bodyHTML != "" && bodyText != "" {
			// Multipart alternative
			boundary := fmt.Sprintf("----=_Part_%d", time.Now().UnixNano())
			writeHeader(&buf, "MIME-Version", "1.0")
			writeHeader(&buf, "Content-Type", fmt.Sprintf("multipart/alternative; boundary=\"%s\"", boundary))
			buf.WriteString("\r\n")

			// Plain text part
			buf.WriteString("--" + boundary + "\r\n")
			buf.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
			buf.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
			buf.WriteString(bodyText)
			buf.WriteString("\r\n")

			// HTML part
			buf.WriteString("--" + boundary + "\r\n")
			buf.WriteString("Content-Type: text/html; charset=utf-8\r\n")
			buf.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
			buf.WriteString(bodyHTML)
			buf.WriteString("\r\n")

			buf.WriteString("--" + boundary + "--\r\n")
		} else if bodyHTML != "" {
			writeHeader(&buf, "MIME-Version", "1.0")
			writeHeader(&buf, "Content-Type", "text/html; charset=utf-8")
			buf.WriteString("\r\n")
			buf.WriteString(bodyHTML)
		} else {
			writeHeader(&buf, "MIME-Version", "1.0")
			writeHeader(&buf, "Content-Type", "text/plain; charset=utf-8")
			buf.WriteString("\r\n")
			buf.WriteString(bodyText)
		}
	}

	return buf.Bytes(), strings.Trim(messageID, "<>"), msgDate
}

// writeHeader writes a header line to the buffer
func writeHeader(buf *bytes.Buffer, name, value string) {
	if value != "" {
		buf.WriteString(name)
		buf.WriteString(": ")
		buf.WriteString(value)
		buf.WriteString("\r\n")
	}
}

// encodeHeader encodes a header value if it contains non-ASCII characters
func encodeHeader(s string) string {
	needsEncoding := false
	for _, r := range s {
		if r > 127 {
			needsEncoding = true
			break
		}
	}
	if needsEncoding {
		return mime.QEncoding.Encode("utf-8", s)
	}
	return s
}

// formatAddress formats an email address with optional display name
func formatAddress(name, email string) string {
	if name == "" {
		return email
	}
	if email == "" {
		return name
	}
	return fmt.Sprintf("%s <%s>", encodeHeader(name), email)
}
