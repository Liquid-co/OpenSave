package cloud

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/opensave/opensave/internal/store"
)

// localFolder keeps snapshots in a folder on this machine or a mounted share
// (a NAS, a synced folder another program uploads).
type localFolder struct {
	cfg store.CloudConfig
}

func (p localFolder) upload(f *os.File, size int64, fileName string) error {
	if p.cfg.URL == "" {
		return fmt.Errorf("no local folder destination configured")
	}
	if err := os.MkdirAll(p.cfg.URL, 0o777); err != nil {
		return err
	}
	out, err := os.Create(filepath.Join(p.cfg.URL, fileName))
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, f); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func (p localFolder) list() ([]CloudFile, error) {
	if p.cfg.URL == "" {
		return []CloudFile{}, nil
	}
	entries, err := os.ReadDir(p.cfg.URL)
	if err != nil {
		return []CloudFile{}, nil
	}
	var files []CloudFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".zip") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, CloudFile{
			Name: e.Name(), SizeBytes: info.Size(),
			CreatedTime: info.ModTime().UTC().Format(time.RFC3339),
		})
	}
	return files, nil
}

func (p localFolder) download(fileName, localPath string) error {
	if p.cfg.URL == "" {
		return fmt.Errorf("no local folder destination configured")
	}
	src, err := os.Open(filepath.Join(p.cfg.URL, fileName))
	if err != nil {
		return fmt.Errorf("file %q not found in local folder", fileName)
	}
	defer src.Close()
	if err := os.MkdirAll(filepath.Dir(localPath), 0o777); err != nil {
		return err
	}
	out, err := os.Create(localPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, src); err != nil {
		out.Close()
		os.Remove(localPath)
		return err
	}
	return out.Close()
}

func (p localFolder) remove(f CloudFile) error {
	if p.cfg.URL == "" {
		return fmt.Errorf("no local folder destination configured")
	}
	return os.Remove(filepath.Join(p.cfg.URL, f.Name))
}
