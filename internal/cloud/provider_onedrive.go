package cloud

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// oneDrive keeps snapshots in the app's own folder of the signed-in
// account's OneDrive (Apps/OpenSave), through Microsoft Graph.
type oneDrive struct {
	s *Service
}

func (p oneDrive) token() (string, error) {
	return p.s.getOrRefreshAccessToken("onedrive")
}

// itemURL is the Graph address of one file in the app folder.
func (p oneDrive) itemURL(fileName string) string {
	return p.s.Endpoints.Graph + "/v1.0/me/drive/special/approot:/" + url.PathEscape(fileName)
}

func (p oneDrive) upload(f *os.File, size int64, fileName string) error {
	token, err := p.token()
	if err != nil {
		return err
	}
	if size > onedriveSimpleLimit {
		err = p.uploadSession(token, fileName, f, size)
	} else {
		err = p.uploadSimple(token, fileName, f, size)
	}
	if err != nil {
		return fmt.Errorf("OneDrive: %w", err)
	}
	return nil
}

// uploadSimple streams one PUT (fine below ~4 MB).
func (p oneDrive) uploadSimple(token, fileName string, f *os.File, size int64) error {
	req, err := http.NewRequest(http.MethodPut, p.itemURL(fileName)+":/content", f)
	if err != nil {
		return err
	}
	req.ContentLength = size
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/zip")
	resp, err := p.s.doTransfer(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return transferOK(resp)
}

// uploadSession uses Graph upload sessions: chunks must be multiples of
// 320 KiB and go to a pre-authorized URL (no auth header).
func (p oneDrive) uploadSession(token, fileName string, f *os.File, size int64) error {
	body, _ := json.Marshal(map[string]any{
		"item": map[string]any{"@microsoft.graph.conflictBehavior": "replace"},
	})
	req, err := http.NewRequest(http.MethodPost, p.itemURL(fileName)+":/createUploadSession", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.s.doTransfer(req)
	if err != nil {
		return err
	}
	var session struct {
		UploadURL string `json:"uploadUrl"`
	}
	if err := transferOK(resp); err != nil {
		resp.Body.Close()
		return fmt.Errorf("create upload session: %w", err)
	}
	err = json.NewDecoder(resp.Body).Decode(&session)
	resp.Body.Close()
	if err != nil || session.UploadURL == "" {
		return fmt.Errorf("create upload session: no uploadUrl")
	}

	for offset := int64(0); offset < size; {
		n := onedriveChunkSize
		if remaining := size - offset; remaining < n {
			n = remaining
		}
		req, err := http.NewRequest(http.MethodPut, session.UploadURL, io.NewSectionReader(f, offset, n))
		if err != nil {
			return err
		}
		req.ContentLength = n
		req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, offset+n-1, size))
		resp, err := p.s.doTransfer(req)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusAccepted &&
			resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			err := transferOK(resp)
			resp.Body.Close()
			return fmt.Errorf("upload chunk at %d: %w", offset, err)
		}
		resp.Body.Close()
		offset += n
	}
	return nil
}

func (p oneDrive) list() ([]CloudFile, error) {
	token, err := p.token()
	if err != nil {
		return nil, err
	}
	// OneDrive pages by handing back the next page's URL.
	next := p.s.Endpoints.Graph + "/v1.0/me/drive/special/approot/children"
	var files []CloudFile
	for page := 0; page < maxListPages && next != ""; page++ {
		req, _ := http.NewRequest(http.MethodGet, next, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		var out struct {
			Value []struct {
				Name            string          `json:"name"`
				Size            int64           `json:"size"`
				CreatedDateTime string          `json:"createdDateTime"`
				File            json.RawMessage `json:"file"`
			} `json:"value"`
			NextLink string `json:"@odata.nextLink"`
		}
		if err := p.s.doJSON(req, &out); err != nil {
			return nil, fmt.Errorf("OneDrive: %w", err)
		}
		for _, f := range out.Value {
			if f.File != nil && strings.HasSuffix(f.Name, ".zip") {
				files = append(files, CloudFile{Name: f.Name, SizeBytes: f.Size, CreatedTime: f.CreatedDateTime})
			}
		}
		if out.NextLink == next {
			return nil, errListTooLong("OneDrive")
		}
		next = out.NextLink
	}
	if next != "" {
		return nil, errListTooLong("OneDrive")
	}
	return files, nil
}

func (p oneDrive) download(fileName, localPath string) error {
	token, err := p.token()
	if err != nil {
		return err
	}
	req, _ := http.NewRequest(http.MethodGet, p.itemURL(fileName)+":/content", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	if err := p.s.fetchToFile(req, localPath); err != nil {
		return fmt.Errorf("OneDrive: %w", err)
	}
	return nil
}

func (p oneDrive) remove(f CloudFile) error {
	token, err := p.token()
	if err != nil {
		return err
	}
	req, _ := http.NewRequest(http.MethodDelete, p.itemURL(f.Name), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	if err := p.s.doOK(req); err != nil {
		return fmt.Errorf("OneDrive: %w", err)
	}
	return nil
}
