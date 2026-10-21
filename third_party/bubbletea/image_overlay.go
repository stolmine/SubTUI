package tea

import (
	"bytes"
	"slices"
	"sync"

	"github.com/charmbracelet/x/ansi"
)

// SubTUI patch: terminal image overlays.
//
// The renderer rewrites whole lines, which in terminals that attach images
// to cells (WezTerm, iTerm2) erases any image on that line. An ImageOverlay
// marks a rectangle the renderer must not paint over: lines crossing it are
// written around it, and the image itself is only re-sent when the overlay
// changes or the screen is fully repainted. Only used on the alt screen.

// ImageOverlay is an image drawn at a fixed cell rectangle of the view.
type ImageOverlay struct {
	Key  string // Identifies the image content and size; a new Key is redrawn
	Row  int    // Top row, 0-based
	Col  int    // Left column, 0-based
	Rows int
	Cols int
	Seq  string // Escape sequence that draws the image at the cursor
}

var (
	overlayMu       sync.Mutex
	overlays        []ImageOverlay
	overlaysChanged bool
)

// SetImageOverlays replaces the image overlays for the next frame. Call it
// from View; passing the same overlays again is cheap and redraws nothing.
func SetImageOverlays(o []ImageOverlay) {
	overlayMu.Lock()
	defer overlayMu.Unlock()

	if slices.EqualFunc(o, overlays, func(a, b ImageOverlay) bool {
		return a.Key == b.Key && a.Row == b.Row && a.Col == b.Col && a.Rows == b.Rows && a.Cols == b.Cols
	}) {
		return
	}

	overlays = slices.Clone(o)
	overlaysChanged = true
}

// takeOverlays returns the current overlays and whether they changed since
// the last call.
func takeOverlays() ([]ImageOverlay, bool) {
	overlayMu.Lock()
	defer overlayMu.Unlock()

	changed := overlaysChanged
	overlaysChanged = false
	return overlays, changed
}

// overlayAt returns the overlay covering row, if any.
func overlayAt(o []ImageOverlay, row int) (ImageOverlay, bool) {
	for _, ov := range o {
		if row >= ov.Row && row < ov.Row+ov.Rows {
			return ov, true
		}
	}
	return ImageOverlay{}, false
}

// writeOverlays draws each overlay's image at its position, inside a
// synchronized update so the terminal shows the frame at once.
func writeOverlays(buf *bytes.Buffer, o []ImageOverlay) {
	buf.WriteString(ansi.SetSynchronizedOutputMode)
	for _, ov := range o {
		buf.WriteString(ansi.CursorPosition(ov.Col+1, ov.Row+1))
		buf.WriteString(ov.Seq)
	}
	buf.WriteString(ansi.ResetSynchronizedOutputMode)
}
