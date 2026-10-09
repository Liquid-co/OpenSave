package daemon

import (
	"fmt"
	"strings"
	"time"

	"github.com/opensave/opensave/internal/cloud"
	"github.com/opensave/opensave/internal/snapshot"
	"github.com/opensave/opensave/internal/store"
)

// maxRetriesPerCheck bounds how many failed uploads one cloud check sends
// again, so a long outage's backlog goes up over a few checks rather than
// holding one for as long as all of it takes.
const maxRetriesPerCheck = 10

// noteUploadFailed keeps a snapshot whose cloud copy did not go up, to be sent
// again by retryFailedUploads.
func (d *Daemon) noteUploadFailed(zipPath, remoteName string) {
	gameID, _, _, ok := snapshot.ParseExportEntryName(remoteName)
	if !ok {
		return
	}
	if err := d.Store.NoteCloudUploadFailed(gameID, remoteName, zipPath, time.Now().UnixMilli()); err != nil {
		d.Log.Log("warn", err.Error())
	}
}

// retryFailedUploads sends again the cloud copies that failed to go up. It
// runs from the cloud check once a listing has worked, which is the proof
// that the provider can be reached again. Each is sent once per check at
// most, and forgotten once it is there — sent by this, by hand, or by another
// path — or once the snapshot is gone here.
func (d *Daemon) retryFailedUploads(inCloud []cloud.CloudFile) {
	pending, err := d.Store.CloudRetries()
	if err != nil || len(pending) == 0 {
		return
	}
	present := make(map[string]bool, len(inCloud))
	for _, f := range inCloud {
		present[f.Name] = true
	}
	var send []store.CloudRetry
	for _, p := range pending {
		if present[p.RemoteName] || !snapshot.ArchiveExists(p.ZipPath) {
			_ = d.Store.ForgetCloudRetry(p.RemoteName)
			continue
		}
		send = append(send, p)
	}
	if len(send) == 0 {
		return
	}
	if len(send) > maxRetriesPerCheck {
		send = send[:maxRetriesPerCheck]
	}
	what := "1 snapshot"
	if len(send) > 1 {
		what = fmt.Sprintf("%d snapshots", len(send))
	}
	d.Log.Log("info", "cloud: sending again "+what+" that did not go up earlier")
	for _, p := range send {
		if id, _, ok := strings.Cut(p.RemoteName, "__"); ok && d.provisioningBlocks(id) {
			continue
		}
		d.uploads.Add()
		d.runCloudUpload(p.ZipPath, p.RemoteName, d.Log)
	}
}
