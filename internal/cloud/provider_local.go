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
	return p.put(f, fileName)
}

// put stores what r holds under fileName, appearing under that name only
// once it is all there (see writeFileWhole). Another device reading the same
// folder never lists a snapshot that is still being copied, and a copy that
// fails leaves nothing that looks like one.
func (p localFolder) put(r io.Reader, fileName string) error {
	if p.cfg.URL == "" {
		return fmt.Errorf("no local folder destination configured")
	}
	return writeFileWhole(filepath.Join(p.cfg.URL, fileName), func(w io.Writer) error {
		_, err := io.Copy(w, r)
		return err
	})
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
	return writeFileWhole(localPath, func(w io.Writer) error {
		_, err := io.Copy(w, src)
		return err
	})
}

func (p localFolder) remove(f CloudFile) error {
	if p.cfg.URL == "" {
		return fmt.Errorf("no local folder destination configured")
	}
	return os.Remove(filepath.Join(p.cfg.URL, f.Name))
}
