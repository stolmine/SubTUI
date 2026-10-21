package ui

import (
	"github.com/MattiaPun/SubTUI/v2/internal/integration"
	tea "github.com/charmbracelet/bubbletea"
)

// Update invalidates the pane cache after every message except position
// ticks that kept the same song: those only change playerStatus, which the
// cached panes (header, sidebar, main list) don't read.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	prevSong, prevIndex, prevLen := m.lastPlayedSongPath, m.queueIndex, len(m.queue)

	res, cmd := m.update(msg)

	if next, ok := res.(model); ok && next.paneCache != nil {
		_, tick := msg.(positionTickMsg)
		if !tick || next.lastPlayedSongPath != prevSong || next.queueIndex != prevIndex || len(next.queue) != prevLen {
			next.paneCache.valid = false
		}
		return next, cmd
	}

	return res, cmd
}

func (m model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		return m.handleWindowResize(msg)

	case tea.KeyMsg:
		return m.handlesKeys(msg)

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case loginResultMsg:
		return m.handleLoginResult(msg)

	case playlistResultMsg:
		return m.handlePlaylistResult(msg)

	case errMsg:
		return m.handleErr(msg)

	case playerChangeMsg:
		return m.handlePlayerChange(msg)

	case positionTickMsg:
		return m.handlePositionTick(msg)

	case songsResultMsg:
		return m.handleSongResult(msg)

	case albumsResultMsg:
		return m.handleAlbumResult(msg)

	case artistsResultMsg:
		return m.handleArtistsResult(msg)

	case starredResultMsg:
		return m.handleStarredResult(msg)

	case viewStarredSongsMsg:
		return m.handleViewStarredSongs(msg)

	case coverArtMsg:
		return m.handleCoverArt(msg)

	case autoDJSeedMsg:
		return m.handleAutoDJSeed(msg)

	case autoDJRefillMsg:
		return m.handleAutoDJRefill(msg)

	case autoDJErrMsg:
		return m.handleAutoDJErr(msg)

	case scrobbleResultMsg:
		return m.handleScrobbleResult(msg)

	case randomAlbumMsg:
		return m.handleRandomAlbum(msg)

	case shuffledSongsMsg:
		return m.handleShuffledSongs(msg)

	case createShareMsg:
		return m.handleCreateShare(msg)

	case getLyricsMsg:
		return m.handleLyrics(msg)

	case playQueueResultMsg:
		return m.handlePlayQueueResult(msg)

	case SetDBusMsg:
		return m.handleSetDBUS(msg)

	case integration.PlayPauseMsg:
		return m.handleIntegrationPlayPause(msg)

	case integration.PlayMsg:
		return m.handleIntegrationPlay()

	case integration.PauseMsg:
		return m.handleIntegrationPause()

	case integration.StopMsg:
		return m.handleIntegrationStop()

	case integration.NextSongMsg:
		return m.handleIntegrationNextSong(msg)

	case integration.PreviousSongMsg:
		return m.handleIntegrationPreviousSong(msg)

	case integration.SetPositionMsg:
		return m.handleIntegrationSetPosition(msg)

	case SetDiscordMsg:
		return m.handleSetDiscord(msg)

	}

	return m, cmd
}
