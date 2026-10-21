package ui

import (
	"math"
	"time"

	"github.com/MattiaPun/SubTUI/v2/internal/player"
	tea "github.com/charmbracelet/bubbletea"
)

// waitPlayerChangeCmd blocks until mpv reports a state change. It is
// re-armed after every playerChangeMsg, so exactly one waiter is live.
func waitPlayerChangeCmd() tea.Cmd {
	return func() tea.Msg {
		<-player.Changes()
		return playerChangeMsg(player.GetPlayerStatus())
	}
}

// schedulePositionTick (re)starts the progress tick. Ticks only run while
// playing and fire just after the next whole second of playback, so the
// clock advances smoothly with one wakeup per second. Bumping the
// generation drops any tick that is already in flight.
func (m *model) schedulePositionTick() tea.Cmd {
	m.positionTickGen++
	if !isPlaying(m.playerStatus) {
		return nil
	}

	gen := m.positionTickGen
	_, frac := math.Modf(m.playerStatus.Current)
	delay := time.Duration((1-frac)*float64(time.Second)) + 20*time.Millisecond

	return tea.Tick(delay, func(time.Time) tea.Msg {
		return positionTickMsg{gen: gen, status: player.GetPlayerStatus()}
	})
}
