//go:build !windows

package main

import (
	"errors"
	goruntime "runtime"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Outside Windows, the system's own notifications through Wails: the title
// and the text, and a click that brings OpenSave up where it is about. On a
// Mac the person is asked once whether OpenSave may send them.

var errNotificationsUnavailable = errors.New("desktop notifications are unavailable here")

func (a *App) initNotifications() {
	if err := runtime.InitializeNotifications(a.ctx); err != nil {
		a.notifyErr = err
		return
	}
	runtime.OnNotificationResponse(a.ctx, func(r runtime.NotificationResult) {
		if r.Error != nil {
			return
		}
		open, _ := r.Response.UserInfo["open"].(string)
		a.openFromNotification(open)
	})
	if goruntime.GOOS == "darwin" {
		go func() { _, _ = runtime.RequestNotificationAuthorization(a.ctx) }()
	}
}

func (a *App) showNote(n DesktopNote) error {
	if a.notifyErr != nil {
		return a.notifyErr
	}
	if !runtime.IsNotificationAvailable(a.ctx) {
		return errNotificationsUnavailable
	}
	return runtime.SendNotification(a.ctx, runtime.NotificationOptions{
		ID:    uuid.NewString(),
		Title: n.Title,
		Body:  n.Body,
		Data:  map[string]interface{}{"open": encodeOpen(n.Open)},
	})
}
