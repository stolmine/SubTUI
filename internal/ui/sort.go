package ui

import (
	"cmp"
	"fmt"
	"math/rand"
	"slices"
	"strings"
	"unicode"

	"github.com/MattiaPun/SubTUI/v2/internal/api"
	"github.com/charmbracelet/lipgloss"
)

// Sorting, modelled on yazi: the sort key (default ,) opens a which-key
// popup, then a letter picks the field; lowercase sorts ascending and
// uppercase in reverse. Ties fall back to a fixed chain ending in the
// server's original order (the Pos each item was stamped with on arrival),
// so equal items keep their order and "o" restores it.

const sortOriginal = "" // listSort.by for the server's order

type listSort struct {
	by      string // sortField.name, or sortOriginal
	reverse bool
}

type sortField struct {
	key  rune
	name string
}

var (
	songSortFields = []sortField{
		{'t', "title"}, {'a', "artist"}, {'l', "album"}, {'n', "track"},
		{'y', "year"}, {'d', "duration"}, {'p', "plays"}, {'r', "random"},
	}
	albumSortFields = []sortField{
		{'t', "name"}, {'a', "artist"}, {'y', "year"}, {'s', "songs"},
		{'d', "duration"}, {'r', "random"},
	}
	artistSortFields = []sortField{
		{'t', "name"}, {'c', "albums"}, {'r', "random"},
	}
)

// sortTarget returns the fields and current sort for the list in the main
// view; ok is false where sorting doesn't apply (e.g. the queue)
func (m *model) sortTarget() (fields []sortField, current *listSort, ok bool) {
	switch m.displayMode {
	case displaySongs:
		if m.viewMode != viewList {
			return nil, nil, false
		}
		return songSortFields, &m.songSort, true
	case displayAlbums:
		return albumSortFields, &m.albumSort, true
	case displayArtist:
		return artistSortFields, &m.artistSort, true
	}
	return nil, nil, false
}

func openSortPopup(m model) model {
	if _, _, ok := m.sortTarget(); ok {
		m.sortPending = true
	}
	return m
}

// handleSortKey applies the key pressed after the sort prefix; any key that
// isn't a sort letter just closes the popup
func handleSortKey(m model, key string) model {
	m.sortPending = false

	fields, current, ok := m.sortTarget()
	runes := []rune(key)
	if !ok || len(runes) != 1 {
		return m
	}

	r := runes[0]
	next := listSort{reverse: unicode.IsUpper(r)}
	lower := unicode.ToLower(r)

	if lower == 'o' {
		next.by = sortOriginal
	} else {
		i := slices.IndexFunc(fields, func(f sortField) bool { return f.key == lower })
		if i < 0 {
			return m
		}
		next.by = fields[i].name
		if next.by == "random" {
			next.reverse = false
		}
	}

	*current = next
	return m.resortKeepingCursor()
}

// resortKeepingCursor sorts the visible list and keeps the cursor on the
// item it was on, like yazi keeps the hovered file
func (m model) resortKeepingCursor() model {
	before := m.cursorItemPos()
	m.applySorts()
	m = resetSelection(m)

	if before < 0 {
		return m
	}

	for i := 0; i < _getMainListLength(m); i++ {
		if m.itemPosAt(i) == before {
			m.cursorMain = i
			break
		}
	}

	visible := _getMainVisibleRows(m)
	if m.cursorMain < m.mainOffset {
		m.mainOffset = m.cursorMain
	} else if m.cursorMain >= m.mainOffset+visible {
		m.mainOffset = m.cursorMain - visible + 1
	}

	return m
}

func (m model) cursorItemPos() int {
	if !cursorInBounds(m) {
		return -1
	}
	return m.itemPosAt(m.cursorMain)
}

func (m model) itemPosAt(i int) int {
	switch m.displayMode {
	case displaySongs:
		return m.songs[i].Pos
	case displayAlbums:
		return m.albums[i].Pos
	case displayArtist:
		return m.artists[i].Pos
	}
	return -1
}

// applySorts puts every list in its current sort order
func (m *model) applySorts() {
	sortList(m.songs, m.songSort, compareSongs)
	sortList(m.albums, m.albumSort, compareAlbums)
	sortList(m.artists, m.artistSort, compareArtists)
}

// stamp records each item's position in the server's order; base is the
// number of items already loaded when paging
func stampSongs(songs []api.Song, base int) {
	for i := range songs {
		songs[i].Pos = base + i
	}
}

func stampAlbums(albums []api.Album, base int) {
	for i := range albums {
		albums[i].Pos = base + i
	}
}

func stampArtists(artists []api.Artist, base int) {
	for i := range artists {
		artists[i].Pos = base + i
	}
}

type positioned interface {
	api.Song | api.Album | api.Artist
}

func sortList[T positioned](items []T, s listSort, compare func(by string, a, b T) int) {
	if s.by == "random" {
		rand.Shuffle(len(items), func(i, j int) { items[i], items[j] = items[j], items[i] })
		return
	}

	slices.SortStableFunc(items, func(a, b T) int {
		c := compare(s.by, a, b)
		if s.reverse {
			return -c
		}
		return c
	})
}

// compareSongs orders by the field, then artist, album, disc, track, title
// and finally the original order
func compareSongs(by string, a, b api.Song) int {
	var c int
	switch by {
	case "title":
		c = natCompare(a.Title, b.Title)
	case "artist":
		c = natCompare(a.Artist, b.Artist)
	case "album":
		c = natCompare(a.Album, b.Album)
	case "track":
		c = cmp.Or(cmp.Compare(a.DiscNumber, b.DiscNumber), cmp.Compare(a.TrackNumber, b.TrackNumber))
	case "year":
		c = cmp.Compare(a.Year, b.Year)
	case "duration":
		c = cmp.Compare(a.Duration, b.Duration)
	case "plays":
		c = cmp.Compare(a.PlayCount, b.PlayCount)
	}

	if by == sortOriginal {
		return cmp.Compare(a.Pos, b.Pos)
	}

	return cmp.Or(c,
		natCompare(a.Artist, b.Artist),
		natCompare(a.Album, b.Album),
		cmp.Compare(a.DiscNumber, b.DiscNumber),
		cmp.Compare(a.TrackNumber, b.TrackNumber),
		natCompare(a.Title, b.Title),
		cmp.Compare(a.Pos, b.Pos),
	)
}

// compareAlbums orders by the field, then artist, year, name and finally
// the original order
func compareAlbums(by string, a, b api.Album) int {
	var c int
	switch by {
	case "name":
		c = natCompare(a.Name, b.Name)
	case "artist":
		c = natCompare(a.Artist, b.Artist)
	case "year":
		c = cmp.Compare(a.Year, b.Year)
	case "songs":
		c = cmp.Compare(a.SongCount, b.SongCount)
	case "duration":
		c = cmp.Compare(a.Duration, b.Duration)
	}

	if by == sortOriginal {
		return cmp.Compare(a.Pos, b.Pos)
	}

	return cmp.Or(c,
		natCompare(a.Artist, b.Artist),
		cmp.Compare(a.Year, b.Year),
		natCompare(a.Name, b.Name),
		cmp.Compare(a.Pos, b.Pos),
	)
}

func compareArtists(by string, a, b api.Artist) int {
	var c int
	switch by {
	case "name":
		c = natCompare(a.Name, b.Name)
	case "albums":
		c = cmp.Compare(a.AlbumCount, b.AlbumCount)
	}

	if by == sortOriginal {
		return cmp.Compare(a.Pos, b.Pos)
	}

	return cmp.Or(c, natCompare(a.Name, b.Name), cmp.Compare(a.Pos, b.Pos))
}

// natCompare compares case-insensitively, with runs of digits compared by
// value ("Track 2" < "Track 10"), like yazi's natural sort
func natCompare(a, b string) int {
	ra, rb := []rune(strings.ToLower(a)), []rune(strings.ToLower(b))
	i, j := 0, 0

	for i < len(ra) && j < len(rb) {
		if unicode.IsDigit(ra[i]) && unicode.IsDigit(rb[j]) {
			si, sj := i, j
			for i < len(ra) && unicode.IsDigit(ra[i]) {
				i++
			}
			for j < len(rb) && unicode.IsDigit(rb[j]) {
				j++
			}

			na := strings.TrimLeft(string(ra[si:i]), "0")
			nb := strings.TrimLeft(string(rb[sj:j]), "0")
			if c := cmp.Or(cmp.Compare(len(na), len(nb)), strings.Compare(na, nb)); c != 0 {
				return c
			}
			continue
		}

		if c := cmp.Compare(ra[i], rb[j]); c != 0 {
			return c
		}
		i++
		j++
	}

	return cmp.Compare(len(ra)-i, len(rb)-j)
}

// sortLabel is the indicator for the main view border, e.g. "year ↓"
func (m model) sortLabel() string {
	_, current, ok := m.sortTarget()
	if !ok || current.by == sortOriginal && !current.reverse {
		return ""
	}

	name := current.by
	if name == sortOriginal {
		name = "original"
	}
	if name == "random" {
		return name
	}
	if current.reverse {
		return name + " ↓"
	}
	return name + " ↑"
}

// sortPopupContent is the which-key style list shown after the sort key
func (m model) sortPopupContent() string {
	fields, current, _ := m.sortTarget()

	entry := func(key rune, name string) string {
		keys := fmt.Sprintf("%c %c", key, unicode.ToUpper(key))
		if name == "random" {
			keys = fmt.Sprintf("%c  ", key)
		}

		line := fmt.Sprintf("%s  %s", keys, name)
		active := current.by == name || (name == "original" && current.by == sortOriginal)
		if active {
			return highlightStyle.Bold(true).Render(line)
		}
		return line
	}

	lines := []string{lipgloss.NewStyle().Bold(true).Render("Sort"), ""}
	for _, f := range fields {
		lines = append(lines, entry(f.key, f.name))
	}
	lines = append(lines, entry('o', "original"), "", subtleStyle.Render("lowercase ↑ · uppercase ↓"))

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}
