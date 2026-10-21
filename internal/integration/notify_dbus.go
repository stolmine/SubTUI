//go:build linux || freebsd

package integration

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/esiqveland/notify"
	"github.com/godbus/dbus/v5"
)

const notificationTimeout = 5 * time.Second

var (
	notifyMu     sync.Mutex
	notifier     notify.Notifier
	lastNotifyID uint32
	clickCommand string
)

// SetNotificationClickCommand sets the shell command run when a track
// notification is clicked. Empty means the notification offers no action.
func SetNotificationClickCommand(cmd string) {
	notifyMu.Lock()
	clickCommand = cmd
	notifyMu.Unlock()
}

// NotifyTrack shows a track change notification. Each call replaces the
// previous one, and it is marked transient so track changes don't pile up
// in the notification history.
func NotifyTrack(summary, body string, art []byte) error {
	notifyMu.Lock()
	defer notifyMu.Unlock()

	if notifier == nil {
		n, err := newNotifier()
		if err != nil {
			return err
		}
		notifier = n
	}

	note := notify.Notification{
		AppName:       "SubTUI",
		ReplacesID:    lastNotifyID,
		Summary:       summary,
		Body:          body,
		ExpireTimeout: notificationTimeout,
	}
	note.AddHint(notify.Hint{ID: "transient", Variant: dbus.MakeVariant(true)})
	note.AddHint(notify.Hint{ID: "category", Variant: dbus.MakeVariant("x-subtui.track")})

	if img := decodeRGBA(art); img != nil {
		note.AddHint(notify.HintImageDataRGBA(img))
	}

	if clickCommand != "" {
		note.Actions = []notify.Action{notify.NewDefaultAction("Show SubTUI")}
	}

	id, err := notifier.SendNotification(note)
	if err != nil {
		return err
	}
	lastNotifyID = id

	return nil
}

// newNotifier uses a private bus connection, so its signal handling and
// Close don't interfere with the shared MPRIS connection
func newNotifier() (notify.Notifier, error) {
	conn, err := dbus.SessionBusPrivate()
	if err != nil {
		return nil, err
	}
	if err := conn.Auth(nil); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := conn.Hello(); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return notify.New(conn, notify.WithOnAction(onNotificationAction))
}

func onNotificationAction(sig *notify.ActionInvokedSignal) {
	notifyMu.Lock()
	ours := sig.ID == lastNotifyID
	cmd := clickCommand
	notifyMu.Unlock()

	if !ours || sig.ActionKey != "default" || cmd == "" {
		return
	}

	c := exec.Command("sh", "-c", cmd)
	c.Env = append(os.Environ(), fmt.Sprintf("SUBTUI_PID=%d", os.Getpid()))
	if err := c.Start(); err != nil {
		log.Printf("[Notify] click command failed: %v", err)
		return
	}
	go func() { _ = c.Wait() }()
}

func decodeRGBA(data []byte) *image.RGBA {
	if len(data) == 0 {
		return nil
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}

	if rgba, ok := img.(*image.RGBA); ok {
		return rgba
	}

	rgba := image.NewRGBA(img.Bounds())
	draw.Draw(rgba, rgba.Bounds(), img, img.Bounds().Min, draw.Src)
	return rgba
}

// CloseNotifier releases the notification bus connection
func CloseNotifier() {
	notifyMu.Lock()
	defer notifyMu.Unlock()

	if notifier != nil {
		_ = notifier.Close()
		notifier = nil
	}
}
