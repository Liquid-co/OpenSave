package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/opensave/opensave/internal/store"
)

// Exercise the interactive handler through daemon registration, not just the
// Store waiter: cancellation must not become a delayed successful mutation.
func TestRegistrationClaimHandlerWaitAndCancellation(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		name := "drains"
		if cancelled {
			name = "cancelled"
		}
		t.Run(name, func(t *testing.T) {
			ts := startTestServer(t)
			a, err := ts.daemon.Store.BeginImportPath(filepath.Join(t.TempDir(), "import"), "incoming")
			if err != nil {
				t.Fatal(err)
			}
			defer a.Release()
			body, _ := json.Marshal(map[string]string{"name": "Claim Wait", "savePath": ts.saveDir})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			req := httptest.NewRequest(http.MethodPost, "/api/games", bytes.NewReader(body)).WithContext(ctx)
			response := httptest.NewRecorder()
			done := make(chan struct{})
			go func() { ts.server.handleTrackGame(response, req); close(done) }()
			select {
			case <-done:
				t.Fatalf("handler refused without waiting: %d %s", response.Code, response.Body)
			case <-time.After(60 * time.Millisecond):
			}
			if cancelled {
				cancel()
			} else {
				a.Release()
			}
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("handler did not finish")
			}
			a.Release()
			_, err = ts.daemon.Store.FindGameBySavePath(ts.saveDir)
			if cancelled {
				if !errors.Is(err, store.ErrNotFound) {
					t.Fatalf("cancelled request registered game: %v", err)
				}
				if response.Code != http.StatusConflict {
					t.Fatalf("busy cancellation HTTP %d: %s", response.Code, response.Body)
				}
			} else if err != nil || response.Code != http.StatusOK {
				t.Fatalf("drained request HTTP %d, game error %v: %s", response.Code, err, response.Body)
			}
		})
	}
}
