package carddav

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/emersion/go-vcard"

	"github.com/mxguardian/pst-import-tool/internal/pst"
)

const (
	CardDAVServer = "https://webmail.mxguardian.net/dav.php/addressbooks/AddressBook/"
)

// Uploader handles uploading contacts to CardDAV
type Uploader struct {
	client   *http.Client
	baseURL  string
	username string
	password string
}

// NewUploader creates a new CardDAV uploader
func NewUploader(username, password string) (*Uploader, error) {
	// Create HTTP client with TLS (skip verify like the Python script)
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	return &Uploader{
		client:   httpClient,
		baseURL:  CardDAVServer,
		username: username,
		password: password,
	}, nil
}

// Upload uploads a single contact to CardDAV via HTTP PUT
func (u *Uploader) Upload(contact *pst.Contact) error {
	// Encode vCard to bytes
	var buf bytes.Buffer
	enc := vcard.NewEncoder(&buf)
	if err := enc.Encode(contact.Card); err != nil {
		return fmt.Errorf("failed to encode vCard: %w", err)
	}

	// Build the full URL for this contact
	url := u.baseURL + contact.UID + ".vcf"

	// Create PUT request
	req, err := http.NewRequest("PUT", url, &buf)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(u.username, u.password)
	req.Header.Set("Content-Type", "text/vcard; charset=utf-8")

	// Execute request
	resp, err := u.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload: %w", err)
	}
	defer resp.Body.Close()

	// Check response - 201 Created or 204 No Content are success
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%d %s", resp.StatusCode, resp.Status)
	}

	return nil
}

// Close is a no-op for CardDAV (HTTP is stateless)
func (u *Uploader) Close() error {
	return nil
}

// TestConnection tests the CardDAV connection with a PROPFIND request
func TestConnection(username, password string) error {
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	req, err := http.NewRequest("PROPFIND", CardDAVServer, nil)
	if err != nil {
		return err
	}

	req.SetBasicAuth(username, password)
	req.Header.Set("Depth", "0")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("CardDAV connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("authentication failed")
	}

	return nil
}
