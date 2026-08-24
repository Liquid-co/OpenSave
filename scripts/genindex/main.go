// Regenerates internal/presets/embedded/ludusavi-index.json.gz from a
// downloaded manifest, so a fresh install carries the same fields a scan
// builds for itself.
package main

import (
	"compress/gzip"
	"fmt"
	"os"

	"github.com/opensave/opensave/internal/presets"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: genidx <manifest.yaml> <out.json.gz>")
		os.Exit(2)
	}
	raw, err := presets.BuildEmbeddedIndexJSON(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "build:", err)
		os.Exit(1)
	}
	f, err := os.Create(os.Args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()
	zw, _ := gzip.NewWriterLevel(f, gzip.BestCompression)
	if _, err := zw.Write(raw); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := zw.Close(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("  wrote %s (%d bytes uncompressed)\n", os.Args[2], len(raw))
}
