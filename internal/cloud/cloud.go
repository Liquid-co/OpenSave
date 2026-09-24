// Package cloud implements snapshot backup to six providers — Google
// Drive, Dropbox, OneDrive, WebDAV, webhook, and a local folder — porting
// src/daemon/cloud.js. API base URLs are injectable so tests can stand in
// httptest servers for every provider.
package cloud

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/opensave/opensave/internal/store"
)

// CloudFile is one remote snapshot entry.
type CloudFile struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name"`
	SizeBytes   int64  `json:"sizeBytes"`
	CreatedTime string `json:"createdTime"`
}

// Endpoints holds provider API bases; override in tests.
type Endpoints struct {
	GoogleAPI      string // https://www.googleapis.com
	GoogleUpload   string // https://www.googleapis.com (upload path shares the host)
	GoogleToken    string // https://oauth2.googleapis.com/token
	GoogleUserInfo string // https://www.googleapis.com/oauth2/v2/userinfo
	DropboxAPI     string // https://api.dropboxapi.com
	DropboxContent string // https://content.dropboxapi.com
	DropboxToken   string // https://api.dropbox.com/oauth2/token
	Graph          string // https://graph.microsoft.com
	MicrosoftToken string // https://login.microsoftonline.com/common/oauth2/v2.0/token
}

// DefaultEndpoints returns the production provider hosts.
func DefaultEndpoints() Endpoints {
	return Endpoints{
		GoogleAPI:      "https://www.googleapis.com",
		GoogleUpload:   "https://www.googleapis.com",
		GoogleToken:    "https://oauth2.googleapis.com/token",
		GoogleUserInfo: "https://www.googleapis.com/oauth2/v2/userinfo",
		DropboxAPI:     "https://api.dropboxapi.com",
		DropboxContent: "https://content.dropboxapi.com",
		DropboxToken:   "https://api.dropbox.com/oauth2/token",
		Graph:          "https://graph.microsoft.com",
		MicrosoftToken: "https://login.microsoftonline.com/common/oauth2/v2.0/token",
	}
}

// Service performs cloud operations against the configured provider.
type Service struct {
	Store     *store.Store
	Log       func(level, msg string)
	Endpoints Endpoints
	HTTP      *http.Client

	driveFolderMu sync.Mutex
	driveFolderID string // cached id of the auto-managed "OpenSave" Drive folder
}

// New creates a production Service.
func New(s *store.Store, logf func(level, msg string)) *Service {
	return &Service{
		Store:     s,
		Log:       logf,
		Endpoints: DefaultEndpoints(),
		HTTP:      &http.Client{Timeout: 60 * time.Second},
	}
}

// IsNotConfigured reports whether err just means cloud backup isn't set up
// (disabled, no destination, or not signed in) — callers like the snapshot
// auto-upload hook skip logging these instead of alarming the user.
func IsNotConfigured(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "not enabled") ||
		strings.Contains(msg, "destination configured") ||
		strings.Contains(msg, "destination URL configured") ||
		strings.Contains(msg, "not authenticated")
}

func (s *Service) config() (store.CloudConfig, error) {
	cfg, err := s.Store.GetCloudConfig()
	if err != nil {
		return store.CloudConfig{}, err
	}
	if !cfg.Enabled {
		return store.CloudConfig{}, fmt.Errorf("cloud sync is not enabled")
	}
	return cfg, nil
}

// ── large-file transfer plumbing ─────────────────────────────────────────
//
// Uploads and downloads stream from/to disk — file size never dictates
// memory use. Providers with small single-request limits switch to their
// chunked/resumable protocols past a threshold, which is what makes
// multi-GB files workable. Thresholds/chunk sizes are vars so tests can
// exercise the chunked paths with small files.

var (
	driveChunkSize          int64 = 16 << 20  // resumable upload chunk (multiple of 256 KiB)
	dropboxSessionThreshold int64 = 128 << 20 // singles are allowed to 150 MB; stay under
	dropboxChunkSize        int64 = 48 << 20
	onedriveSimpleLimit     int64 = 4 << 20  // Graph recommends sessions above 4 MB
	onedriveChunkSize       int64 = 10 << 20 // multiple of 320 KiB
)

// transferClient is used for bulk data movement: no overall client
// timeout (a 60s cap kills large transfers on slow links); each request
// carries its own generous context deadline instead.
func (s *Service) transferClient() *http.Client {
	return &http.Client{Timeout: 0}
}

const perRequestTransferTimeout = 30 * time.Minute

// doTransfer runs one bulk-data request with a per-request deadline and
// returns the response. Caller closes the body.
func (s *Service) doTransfer(req *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(req.Context(), perRequestTransferTimeout)
	resp, err := s.transferClient().Do(req.WithContext(ctx))
	if err != nil {
		cancel()
		return nil, err
	}
	// Tie the cancel to body close so the deadline covers the read.
	resp.Body = &cancelReadCloser{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

type cancelReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelReadCloser) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}

// transferOK drains error info from a non-2xx response.
func transferOK(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	return fmt.Errorf("HTTP %d - %s", resp.StatusCode, strings.TrimSpace(string(raw)))
}

// fetchToFile streams a response body to localPath.
func (s *Service) fetchToFile(req *http.Request, localPath string) error {
	resp, err := s.doTransfer(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := transferOK(resp); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(localPath), 0o777); err != nil {
		return err
	}
	out, err := os.Create(localPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		out.Close()
		os.Remove(localPath)
		return err
	}
	return out.Close()
}

// Upload sends a snapshot zip to the configured provider. Errors are
// returned (the snapshot hook logs them without failing the snapshot).
func (s *Service) Upload(filePath, fileName string) error {
	return s.upload(filePath, fileName, true)
}

// upload is Upload, with the activity log optional: a head is bookkeeping,
// and announcing each one would double every line a snapshot upload writes.
func (s *Service) upload(filePath, fileName string, logged bool) error {
	cfg, err := s.config()
	if err != nil {
		return err
	}

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("file not found: %s", filePath)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	size := info.Size()
	if logged {
		s.Log("info", fmt.Sprintf("cloud: uploading %s (%.1f MB) via %s", fileName, float64(size)/(1<<20), strings.ToUpper(cfg.Provider)))
	}

	p, ok := s.providerFor(cfg)
	if !ok {
		return fmt.Errorf("unsupported cloud sync provider: %s", cfg.Provider)
	}
	if err := p.upload(f, size, fileName); err != nil {
		return err
	}

	if logged {
		s.Log("success", fmt.Sprintf("cloud: uploaded %q to %s", fileName, cfg.Provider))
	}
	return nil
}

// List returns the provider's snapshot zips.
func (s *Service) List() ([]CloudFile, error) {
	cfg, err := s.config()
	if err != nil {
		return nil, err
	}
	p, ok := s.providerFor(cfg)
	if !ok {
		return []CloudFile{}, nil
	}
	return p.list()
}

// maxListPages bounds how many pages one listing will follow: a thousand
// pages is past a million files, so reaching it means a provider handing back
// the same page forever, not a big folder.
const maxListPages = 1000

// errListTooLong reports a listing that never ended. An error rather than
// the pages read so far: a partial listing is what every caller already
// trusted as the whole folder, which is the bug the paging exists to fix.
func errListTooLong(provider string) error {
	return fmt.Errorf("%s: the file listing did not end after %d pages", provider, maxListPages)
}

// Download fetches a remote snapshot to localPath.
func (s *Service) Download(fileName, localPath string) error {
	cfg, err := s.config()
	if err != nil {
		return err
	}
	p, ok := s.providerFor(cfg)
	if !ok {
		return fmt.Errorf("downloading is not supported for provider: %s", cfg.Provider)
	}
	return p.download(fileName, localPath)
}

// Delete removes one remote snapshot. Webhook destinations are fire-and-
// forget and don't support deletion.
func (s *Service) Delete(f CloudFile) error {
	cfg, err := s.config()
	if err != nil {
		return err
	}
	p, ok := s.providerFor(cfg)
	if !ok {
		return fmt.Errorf("deletion is not supported for provider: %s", cfg.Provider)
	}
	return p.remove(f)
}

// PruneGameBranch keeps the newest `keep` remote snapshots of one game
// branch and deletes the rest — the cloud-side mirror of local snapshot
// retention. matchName reports whether a remote file belongs to the
// game+branch (the caller owns the naming scheme). keep <= 0 disables
// pruning.
func (s *Service) PruneGameBranch(matchName func(string) bool, keep int) (int, error) {
	if keep <= 0 {
		return 0, nil
	}
	files, err := s.List()
	if err != nil {
		return 0, err
	}
	var matches []CloudFile
	for _, f := range files {
		if matchName(f.Name) {
			matches = append(matches, f)
		}
	}
	if len(matches) <= keep {
		return 0, nil
	}
	// Newest first; delete everything past `keep`.
	sortByCreatedDesc(matches)
	pruned := 0
	for _, f := range matches[keep:] {
		if err := s.Delete(f); err != nil {
			s.Log("warn", fmt.Sprintf("cloud: prune of %s failed: %v", f.Name, err))
			continue
		}
		pruned++
	}
	if pruned > 0 {
		s.Log("info", fmt.Sprintf("cloud: pruned %d old snapshot(s) beyond retention of %d", pruned, keep))
	}
	return pruned, nil
}

func sortByCreatedDesc(files []CloudFile) {
	for i := 1; i < len(files); i++ {
		for j := i; j > 0 && files[j].CreatedTime > files[j-1].CreatedTime; j-- {
			files[j], files[j-1] = files[j-1], files[j]
		}
	}
}

func (s *Service) httpClient() *http.Client {
	if s.HTTP != nil {
		return s.HTTP
	}
	return http.DefaultClient
}

func (s *Service) doOK(req *http.Request) error {
	resp, err := s.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("HTTP %d - %s", resp.StatusCode, raw)
	}
	return nil
}

func (s *Service) doJSON(req *http.Request, out any) error {
	resp, err := s.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("HTTP %d - %s", resp.StatusCode, raw)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (s *Service) fetchBytes(req *http.Request) ([]byte, error) {
	resp, err := s.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("HTTP %d - %s", resp.StatusCode, raw)
	}
	return io.ReadAll(resp.Body)
}

func joinURL(base, name string) string {
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	return base + name
}

func applyBasicAuth(req *http.Request, username, password string) {
	if username != "" || password != "" {
		cred := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
		req.Header.Set("Authorization", "Basic "+cred)
	}
}

func applyCustomHeaders(req *http.Request, headersJSON string) {
	if headersJSON == "" || headersJSON == "{}" {
		return
	}
	var headers map[string]string
	if err := json.Unmarshal([]byte(headersJSON), &headers); err != nil {
		return
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
}
