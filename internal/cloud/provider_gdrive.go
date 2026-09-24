package cloud

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/opensave/opensave/internal/store"
)

// googleDrive keeps snapshots in a folder of the signed-in account's Drive:
// one named "OpenSave" that it creates, or a folder the person chose.
type googleDrive struct {
	s   *Service
	cfg store.CloudConfig
}

// begin signs in and finds the folder, which every call needs first.
func (p googleDrive) begin() (token, folderID string, err error) {
	token, err = p.s.getOrRefreshAccessToken("google_drive")
	if err != nil {
		return "", "", err
	}
	folderID, err = p.s.driveFolder(p.cfg, token)
	if err != nil {
		return "", "", err
	}
	return token, folderID, nil
}

func (p googleDrive) upload(f *os.File, size int64, fileName string) error {
	token, folderID, err := p.begin()
	if err != nil {
		return err
	}
	if err := p.uploadResumable(token, folderID, fileName, f, size); err != nil {
		return googleDriveErr(err)
	}
	return nil
}

// uploadResumable uses Drive's resumable protocol for every size: one code
// path, streaming chunks, and no request carries more than driveChunkSize
// bytes (multipart uploads are capped at 5 MB by the API).
func (p googleDrive) uploadResumable(token, folderID, fileName string, f *os.File, size int64) error {
	meta, _ := json.Marshal(map[string]any{
		"name": fileName, "mimeType": "application/zip", "parents": []string{folderID},
	})
	initReq, err := http.NewRequest(http.MethodPost,
		p.s.Endpoints.GoogleUpload+"/upload/drive/v3/files?uploadType=resumable", bytes.NewReader(meta))
	if err != nil {
		return err
	}
	initReq.Header.Set("Authorization", "Bearer "+token)
	initReq.Header.Set("Content-Type", "application/json; charset=UTF-8")
	initReq.Header.Set("X-Upload-Content-Type", "application/zip")
	initReq.Header.Set("X-Upload-Content-Length", strconv.FormatInt(size, 10))

	resp, err := p.s.doTransfer(initReq)
	if err != nil {
		return err
	}
	session := resp.Header.Get("Location")
	err = transferOK(resp)
	resp.Body.Close()
	if err != nil {
		return fmt.Errorf("start resumable upload: %w", err)
	}
	if session == "" {
		return fmt.Errorf("resumable upload: no session URL returned")
	}

	for offset := int64(0); offset < size || size == 0; {
		n := driveChunkSize
		if remaining := size - offset; remaining < n {
			n = remaining
		}
		putChunk := func() (*http.Response, error) {
			req, err := http.NewRequest(http.MethodPut, session, io.NewSectionReader(f, offset, n))
			if err != nil {
				return nil, err
			}
			req.ContentLength = n
			req.Header.Set("Content-Range",
				fmt.Sprintf("bytes %d-%d/%d", offset, offset+n-1, size))
			return p.s.doTransfer(req)
		}
		resp, err := putChunk()
		if err != nil || resp.StatusCode >= 500 {
			if resp != nil {
				resp.Body.Close()
			}
			// One retry per chunk — resumable sessions exist for this.
			if resp, err = putChunk(); err != nil {
				return err
			}
		}
		status := resp.StatusCode
		if status != http.StatusOK && status != http.StatusCreated && status != 308 {
			err := transferOK(resp)
			resp.Body.Close()
			return fmt.Errorf("upload chunk at %d: %w", offset, err)
		}
		resp.Body.Close()
		offset += n
		if size == 0 {
			break
		}
	}
	return nil
}

func (p googleDrive) list() ([]CloudFile, error) {
	token, folderID, err := p.begin()
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf("trashed = false and mimeType = 'application/zip' and '%s' in parents", folderID)
	base := p.s.Endpoints.GoogleAPI + "/drive/v3/files?q=" + url.QueryEscape(query) +
		"&pageSize=1000&fields=" + url.QueryEscape("nextPageToken,files(id,name,size,createdTime)")

	// Every page, not the first. Drive answers 100 files at a time unless
	// asked for more, and this read one page: past a hundred snapshots the
	// cloud screens showed an arbitrary hundred of them, a restore could not
	// find the rest, and `cloud push` re-uploaded files it could not see —
	// which on Drive, where a name is not unique, meant duplicates.
	files := []CloudFile{}
	pageToken := ""
	for page := 0; page < maxListPages; page++ {
		listURL := base
		if pageToken != "" {
			listURL += "&pageToken=" + url.QueryEscape(pageToken)
		}
		req, _ := http.NewRequest(http.MethodGet, listURL, nil)
		req.Header.Set("Authorization", "Bearer "+token)

		var out struct {
			NextPageToken string `json:"nextPageToken"`
			Files         []struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				Size        string `json:"size"`
				CreatedTime string `json:"createdTime"`
			} `json:"files"`
		}
		if err := p.s.doJSON(req, &out); err != nil {
			return nil, googleDriveErr(err)
		}
		for _, f := range out.Files {
			size, _ := strconv.ParseInt(f.Size, 10, 64)
			files = append(files, CloudFile{ID: f.ID, Name: f.Name, SizeBytes: size, CreatedTime: f.CreatedTime})
		}
		if out.NextPageToken == "" {
			return files, nil
		}
		if out.NextPageToken == pageToken {
			return nil, errListTooLong("Google Drive")
		}
		pageToken = out.NextPageToken
	}
	return nil, errListTooLong("Google Drive")
}

func (p googleDrive) download(fileName, localPath string) error {
	token, folderID, err := p.begin()
	if err != nil {
		return err
	}
	query := fmt.Sprintf("name = '%s' and trashed = false and '%s' in parents",
		strings.ReplaceAll(fileName, "'", `\'`), folderID)
	listURL := p.s.Endpoints.GoogleAPI + "/drive/v3/files?q=" + url.QueryEscape(query) + "&fields=" + url.QueryEscape("files(id)")
	req, _ := http.NewRequest(http.MethodGet, listURL, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	var out struct {
		Files []struct {
			ID string `json:"id"`
		} `json:"files"`
	}
	if err := p.s.doJSON(req, &out); err != nil {
		return googleDriveErr(err)
	}
	if len(out.Files) == 0 {
		return fmt.Errorf("file %q not found on Google Drive", fileName)
	}
	dlReq, _ := http.NewRequest(http.MethodGet, p.s.Endpoints.GoogleAPI+"/drive/v3/files/"+out.Files[0].ID+"?alt=media", nil)
	dlReq.Header.Set("Authorization", "Bearer "+token)
	if err := p.s.fetchToFile(dlReq, localPath); err != nil {
		return googleDriveErr(err)
	}
	return nil
}

// remove deletes by Drive's file ID, which only a listing supplies: names
// are not unique on Drive.
func (p googleDrive) remove(f CloudFile) error {
	token, err := p.s.getOrRefreshAccessToken("google_drive")
	if err != nil {
		return err
	}
	id := f.ID
	if id == "" {
		return fmt.Errorf("missing Drive file id for %s", f.Name)
	}
	req, _ := http.NewRequest(http.MethodDelete, p.s.Endpoints.GoogleAPI+"/drive/v3/files/"+id, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	if err := p.s.doOK(req); err != nil {
		return googleDriveErr(err)
	}
	return nil
}

// driveFolder returns the Drive folder snapshots live in: the user's
// configured folder ID if set, otherwise a folder named "OpenSave" in the
// Drive root — found or created on first use and cached for the process
// lifetime. Keeps snapshots out of the user's Drive root.
func (s *Service) driveFolder(cfg store.CloudConfig, token string) (string, error) {
	if cfg.FolderID != "" {
		return cfg.FolderID, nil
	}
	s.driveFolderMu.Lock()
	defer s.driveFolderMu.Unlock()
	if s.driveFolderID != "" {
		return s.driveFolderID, nil
	}

	query := "name = 'OpenSave' and mimeType = 'application/vnd.google-apps.folder' and trashed = false and 'root' in parents"
	listURL := s.Endpoints.GoogleAPI + "/drive/v3/files?q=" + url.QueryEscape(query) + "&fields=" + url.QueryEscape("files(id)")
	req, _ := http.NewRequest(http.MethodGet, listURL, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	var out struct {
		Files []struct {
			ID string `json:"id"`
		} `json:"files"`
	}
	if err := s.doJSON(req, &out); err != nil {
		return "", googleDriveErr(err)
	}
	if len(out.Files) > 0 {
		s.driveFolderID = out.Files[0].ID
		return s.driveFolderID, nil
	}

	meta, _ := json.Marshal(map[string]any{
		"name":     "OpenSave",
		"mimeType": "application/vnd.google-apps.folder",
	})
	creq, _ := http.NewRequest(http.MethodPost, s.Endpoints.GoogleAPI+"/drive/v3/files?fields=id", bytes.NewReader(meta))
	creq.Header.Set("Authorization", "Bearer "+token)
	creq.Header.Set("Content-Type", "application/json")
	var created struct {
		ID string `json:"id"`
	}
	if err := s.doJSON(creq, &created); err != nil {
		return "", googleDriveErr(err)
	}
	s.Log("info", `cloud: created "OpenSave" folder in Google Drive`)
	s.driveFolderID = created.ID
	return created.ID, nil
}

// googleDriveErr wraps Drive API failures; a 403 "insufficient permissions"
// means the account was connected without ticking the Drive checkbox on
// Google's consent screen, so tell the user exactly how to fix it.
func googleDriveErr(err error) error {
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "insufficient") && (strings.Contains(msg, "403") || strings.Contains(msg, "permission")) {
		return fmt.Errorf("Google Drive access was not granted for this account — open Cloud Backup, Disconnect, then sign in again and TICK THE CHECKBOX that allows OpenSave to access its own Drive files")
	}
	return fmt.Errorf("Google Drive: %w", err)
}
