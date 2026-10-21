package ui

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"math"
	"os"
	"strings"

	"github.com/MattiaPun/SubTUI/v2/internal/api"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	xdraw "golang.org/x/image/draw"
)

// Album art is drawn either as half-block mosaic text, or as a real image
// using the iTerm2 inline image protocol (WezTerm, iTerm2). For images the
// view renders a blank placeholder whose top-left cell is artSentinel; View
// finds it in the final frame, blanks it, and hands its position to the
// renderer as an image overlay.
const (
	artHalfBlocks = "halfblocks"
	artITerm2     = "iterm2"

	artSentinel = ''

	artFetchSizeImage  = 1200 // px, cover art requested for image mode
	artFetchSizeMosaic = 500
)

var artProtocol = artHalfBlocks

// Terminal cell size in pixels, refreshed on resize; used to keep the
// image box square and to size the encoded image
var cellWidthPx, cellHeightPx = 10.0, 20.0

// artCache holds the encoded image for the last requested size, shared by
// all copies of the model
type artCache struct {
	img        image.Image
	cols, rows int
	key, seq   string
	nextKey    int
}

func detectArtProtocol() string {
	switch p := strings.ToLower(api.AppConfig.Theme.ArtProtocol); p {
	case artHalfBlocks, artITerm2:
		return p
	}

	// auto: multiplexers don't pass inline images through
	if os.Getenv("TMUX") != "" || os.Getenv("ZELLIJ") != "" {
		return artHalfBlocks
	}

	switch os.Getenv("TERM_PROGRAM") {
	case "WezTerm", "iTerm.app":
		return artITerm2
	}
	if os.Getenv("LC_TERMINAL") == "iTerm2" {
		return artITerm2
	}

	return artHalfBlocks
}

func useImageArt() bool {
	return artProtocol == artITerm2
}

func coverArtFetchSize() int {
	if useImageArt() {
		return artFetchSizeImage
	}
	return artFetchSizeMosaic
}

// fitSquareArt returns the largest box (in cells) within maxCols x maxRows
// that is square in pixels
func fitSquareArt(maxCols, maxRows int) (int, int) {
	rows := maxRows
	if byCols := int(float64(maxCols) * cellWidthPx / cellHeightPx); byCols < rows {
		rows = byCols
	}
	cols := int(math.Round(float64(rows) * cellHeightPx / cellWidthPx))
	if cols > maxCols {
		cols = maxCols
	}

	return max(cols, 1), max(rows, 1)
}

// artPlaceholder returns a blank cols x rows block marking where the image
// goes, and makes sure the image is encoded for that size
func (m model) artPlaceholder(cols, rows int) string {
	m.artCache.prepare(m.coverArt, cols, rows)

	line := strings.Repeat(" ", cols)
	lines := make([]string, rows)
	for i := range lines {
		lines[i] = line
	}
	lines[0] = string(artSentinel) + line[1:]

	return strings.Join(lines, "\n")
}

func (c *artCache) prepare(img image.Image, cols, rows int) {
	if c.img == img && c.cols == cols && c.rows == rows {
		return
	}

	c.img, c.cols, c.rows = img, cols, rows
	c.nextKey++
	c.key = fmt.Sprintf("art-%d", c.nextKey)
	c.seq = encodeITerm2(img, cols, rows)
}

// encodeITerm2 scales img to the box's pixel size and wraps it as an
// iTerm2 inline image sized in cells
func encodeITerm2(img image.Image, cols, rows int) string {
	if img == nil {
		return ""
	}

	side := int(math.Min(float64(cols)*cellWidthPx, float64(rows)*cellHeightPx))
	if b := img.Bounds(); side > b.Dx() {
		side = b.Dx()
	}

	scaled := image.NewRGBA(image.Rect(0, 0, side, side))
	xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), img, img.Bounds(), xdraw.Src, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, scaled, &jpeg.Options{Quality: 90}); err != nil {
		return ""
	}

	return fmt.Sprintf("\x1b]1337;File=inline=1;size=%d;width=%d;height=%d;preserveAspectRatio=1;doNotMoveCursor=1:%s\a",
		buf.Len(), cols, rows, base64.StdEncoding.EncodeToString(buf.Bytes()))
}

// placeArt finds the art placeholder in the final frame, blanks its marker
// and registers the image overlay there. With a popup open, no image is
// drawn, so it can't paint over the popup.
func (m model) placeArt(frame string, popupOpen bool) string {
	idx := strings.IndexRune(frame, artSentinel)
	if idx < 0 {
		tea.SetImageOverlays(nil)
		return frame
	}

	frame = frame[:idx] + " " + frame[idx+len(string(artSentinel)):]

	if popupOpen || m.artCache == nil || m.artCache.seq == "" {
		tea.SetImageOverlays(nil)
		return frame
	}

	lineStart := strings.LastIndexByte(frame[:idx], '\n') + 1
	tea.SetImageOverlays([]tea.ImageOverlay{{
		Key:  m.artCache.key,
		Row:  strings.Count(frame[:idx], "\n"),
		Col:  ansi.StringWidth(frame[lineStart:idx]),
		Rows: m.artCache.rows,
		Cols: m.artCache.cols,
		Seq:  m.artCache.seq,
	}})

	return frame
}
