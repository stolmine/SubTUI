//go:build !linux && !freebsd

package integration

import "github.com/gen2brain/beeep"

func SetNotificationClickCommand(cmd string) {}

func NotifyTrack(summary, body string, art []byte) error {
	if len(art) == 0 {
		return beeep.Notify(summary, body, "")
	}
	return beeep.Notify(summary, body, art)
}

func CloseNotifier() {}
