//go:build unix

package ui

import (
	"os"

	"golang.org/x/sys/unix"
)

// updateCellSize reads the terminal's cell size in pixels, when it reports one
func updateCellSize() {
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil || ws.Col == 0 || ws.Row == 0 || ws.Xpixel == 0 || ws.Ypixel == 0 {
		return
	}

	cellWidthPx = float64(ws.Xpixel) / float64(ws.Col)
	cellHeightPx = float64(ws.Ypixel) / float64(ws.Row)
}
