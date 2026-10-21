package ui

import (
	"fmt"
	"math"
	"math/rand"
	"path/filepath"
	"strings"

	"github.com/MattiaPun/SubTUI/v2/internal/api"
	tea "github.com/charmbracelet/bubbletea"
)

// Vector AutoDJ, backed by the recommender at autodj.recommender_url (see
// internal/api/autodj.go). Seeding replaces the queue with recommendations
// for the highlighted item and turns AutoDJ on; while on, each track change
// refills the queue once few enough tracks are left.

const (
	autoDJMaxSeeds     = 50 // an artist's tracks are sampled down to this many seeds
	autoDJRecentWindow = 10 // tracks of context sent with each refill
	autoDJContrastStep = 0.05
)

type autoDJSeedMsg struct {
	lead  *api.Song // the seeded song, played first; nil for album/artist seeds
	songs []api.Song
}

type autoDJRefillMsg struct {
	songs []api.Song
}

type autoDJErrMsg struct {
	err error
}

func autoDJConfigured() bool {
	return api.AppConfig.AutoDJ.RecommenderURL != ""
}

// autoDJStatus is the footer indicator: [DJ off], [DJ 0.35], with … while a
// request is in flight and ! after a failed one
func (m model) autoDJStatus() string {
	if !autoDJConfigured() {
		return ""
	}
	if !api.AppConfig.AutoDJ.Enabled {
		return "[DJ off]"
	}

	status := fmt.Sprintf("[DJ %.2f", api.AppConfig.AutoDJ.Contrast)
	if m.autoDJFetching {
		status += " …"
	} else if m.autoDJFailed {
		status += " !"
	}

	return status + "]"
}

func toggleAutoDJ(m model) (tea.Model, tea.Cmd) {
	if !autoDJConfigured() {
		return m, nil
	}

	api.AppConfig.AutoDJ.Enabled = !api.AppConfig.AutoDJ.Enabled
	m.autoDJFailed = false

	return m, tea.Batch(saveConfigCmd(), m.maybeRefillAutoDJ())
}

func adjustAutoDJContrast(m model, delta float64) (tea.Model, tea.Cmd) {
	if !autoDJConfigured() {
		return m, nil
	}

	c := math.Round((api.AppConfig.AutoDJ.Contrast+delta)/autoDJContrastStep) * autoDJContrastStep
	api.AppConfig.AutoDJ.Contrast = math.Max(0, math.Min(1, c))

	return m, saveConfigCmd()
}

// seedAutoDJ starts a session from the highlighted song, album or artist in
// the main list, or from the playing song elsewhere
func seedAutoDJ(m model) (tea.Model, tea.Cmd) {
	if !autoDJConfigured() {
		return m, nil
	}

	var lead *api.Song
	var albumID, artistID string

	if m.focus == focusMain && !m.showMediaPlayer && cursorInBounds(m) {
		switch m.displayMode {
		case displaySongs:
			list := m.songs
			if m.viewMode == viewQueue {
				list = m.queue
			}
			song := list[m.cursorMain]
			lead = &song
		case displayAlbums:
			albumID = m.albums[m.cursorMain].ID
		case displayArtist:
			artistID = m.artists[m.cursorMain].ID
		}
	} else if len(m.queue) > 0 && m.queueIndex >= 0 && m.queueIndex < len(m.queue) {
		song := m.queue[m.queueIndex]
		lead = &song
	}

	if lead == nil && albumID == "" && artistID == "" {
		return m, nil
	}

	m.autoDJFetching = true
	m.autoDJFailed = false
	cfg := api.AppConfig.AutoDJ

	return m, func() tea.Msg {
		var seeds []string
		switch {
		case lead != nil:
			seeds = []string{lead.ID}
		case albumID != "":
			songs, err := api.SubsonicGetAlbum(albumID)
			if err != nil {
				return autoDJErrMsg{err}
			}
			seeds = songIDs(songs)
		default:
			albums, err := api.SubsonicGetArtist(artistID)
			if err != nil {
				return autoDJErrMsg{err}
			}
			for _, album := range albums {
				if songs, err := api.SubsonicGetAlbum(album.ID); err == nil {
					seeds = append(seeds, songIDs(songs)...)
				}
			}
		}

		if len(seeds) > autoDJMaxSeeds {
			rand.Shuffle(len(seeds), func(i, j int) { seeds[i], seeds[j] = seeds[j], seeds[i] })
			seeds = seeds[:autoDJMaxSeeds]
		}
		if len(seeds) == 0 {
			return autoDJErrMsg{fmt.Errorf("autodj: nothing to seed from")}
		}

		ids, err := api.AutoDJNext(seeds, nil, nil, cfg.SeedBatchSize, cfg.Contrast)
		if err != nil {
			return autoDJErrMsg{err}
		}

		return autoDJSeedMsg{lead: lead, songs: api.SubsonicGetSongs(ids)}
	}
}

// maybeRefillAutoDJ requests more tracks when AutoDJ is on and the queue
// is down to refill_at tracks after the current one
func (m *model) maybeRefillAutoDJ() tea.Cmd {
	cfg := api.AppConfig.AutoDJ
	if !cfg.Enabled || !autoDJConfigured() || m.autoDJFetching {
		return nil
	}
	if len(m.queue) == 0 || m.queueIndex < 0 || m.queueIndex >= len(m.queue) {
		return nil
	}
	if len(m.queue)-m.queueIndex-1 > cfg.RefillAt {
		return nil
	}

	m.autoDJFetching = true

	seeds := []string{m.queue[m.queueIndex].ID}
	recent := songIDs(m.queue[max(0, m.queueIndex+1-autoDJRecentWindow) : m.queueIndex+1])
	exclude := songIDs(m.queue)

	return func() tea.Msg {
		ids, err := api.AutoDJNext(seeds, recent, exclude, cfg.BatchSize, cfg.Contrast)
		if err != nil {
			return autoDJErrMsg{err}
		}

		return autoDJRefillMsg{songs: api.SubsonicGetSongs(ids)}
	}
}

func (m model) handleAutoDJSeed(msg autoDJSeedMsg) (tea.Model, tea.Cmd) {
	m.autoDJFetching = false

	songs := playableSongs(m, msg.songs)
	if msg.lead != nil {
		songs = append([]api.Song{*msg.lead}, songs...)
	}
	if len(songs) == 0 {
		m.autoDJFailed = true
		return m, nil
	}

	api.AppConfig.AutoDJ.Enabled = true
	m.queue = songs

	// Seeded from the song that is already playing: keep it playing and
	// only queue the recommendations after it
	if msg.lead != nil && strings.Contains(m.playerStatus.Path, "id="+msg.lead.ID) {
		m.queueIndex = 0
		m.syncNextSong()
		return m, tea.Batch(m.savePlayQueue(), saveConfigCmd())
	}

	return m, tea.Batch(m.playQueueIndex(0, false), saveConfigCmd())
}

func (m model) handleAutoDJRefill(msg autoDJRefillMsg) (tea.Model, tea.Cmd) {
	m.autoDJFetching = false

	inQueue := make(map[string]bool, len(m.queue))
	for _, song := range m.queue {
		inQueue[song.ID] = true
	}

	added := 0
	for _, song := range playableSongs(m, msg.songs) {
		if !inQueue[song.ID] {
			m.queue = append(m.queue, song)
			inQueue[song.ID] = true
			added++
		}
	}

	if added == 0 {
		m.autoDJFailed = true
		return m, nil
	}

	m.syncNextSong() // the current track may have been the last one
	return m, m.savePlayQueue()
}

func (m model) handleAutoDJErr(msg autoDJErrMsg) (tea.Model, tea.Cmd) {
	m.autoDJFetching = false
	m.autoDJFailed = true
	m.err = msg.err

	return m, nil
}

// playableSongs drops songs excluded by the filters
func playableSongs(m model, songs []api.Song) []api.Song {
	var out []api.Song
	for _, song := range applyExclusionFilters(m, songs) {
		if !song.Filtered {
			out = append(out, song)
		}
	}
	return out
}

func songIDs(songs []api.Song) []string {
	ids := make([]string, 0, len(songs))
	for _, song := range songs {
		ids = append(ids, song.ID)
	}
	return ids
}

// saveConfigCmd persists runtime changes (AutoDJ toggle and contrast) to
// config.toml, from a snapshot taken now
func saveConfigCmd() tea.Cmd {
	cfg := api.AppConfig
	return func() tea.Msg {
		_ = api.SaveConfig(filepath.Join(api.ConfigDir, "config.toml"), cfg, 0644)
		return nil
	}
}
