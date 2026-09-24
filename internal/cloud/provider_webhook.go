package cloud

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"

	"github.com/opensave/opensave/internal/store"
)

// webhook posts each snapshot to a URL and forgets it: the receiving end is
// someone's own server, which may keep the file or pass it on, and offers no
// way to list, fetch or delete it again.
type webhook struct {
	s   *Service
	cfg store.CloudConfig
}

func (p webhook) upload(f *os.File, size int64, fileName string) error {
	if p.cfg.URL == "" {
		return fmt.Errorf("no destination URL configured")
	}
	// Multipart body built on the fly through a pipe — never in RAM.
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		part, err := mw.CreateFormFile("file", fileName)
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		if _, err := io.Copy(part, f); err != nil {
			pw.CloseWithError(err)
			return
		}
		pw.CloseWithError(mw.Close())
	}()
	req, err := http.NewRequest(http.MethodPost, p.cfg.URL, pr)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	applyCustomHeaders(req, p.cfg.HeadersJSON)
	resp, err := p.s.doTransfer(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return transferOK(resp)
}

func (p webhook) list() ([]CloudFile, error) {
	return []CloudFile{}, nil
}

func (p webhook) download(fileName, localPath string) error {
	return fmt.Errorf("downloading is not supported for provider: %s", p.cfg.Provider)
}

func (p webhook) remove(f CloudFile) error {
	return fmt.Errorf("deletion is not supported for provider: %s", p.cfg.Provider)
}
