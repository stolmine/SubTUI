package player

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/MattiaPun/SubTUI/v2/internal/api"
)

var (
	mpvClient *ipcClient
	mpvCmd    *exec.Cmd

	// Player state pushed by mpv (observe_property), so status reads don't
	// need a round-trip per field. Only time-pos is fetched on demand.
	stateMu sync.Mutex
	state   mpvState
	changes = make(chan struct{}, 1)
)

// nilString matches what the UI treats as an unset mpv property
const nilString = "<nil>"

type mpvState struct {
	title, artist, album, path string
	lastPath                   string // last non-nil path, kept across the gap between files
	duration, volume           float64
	paused, idle               bool
}

// Observed properties, by observe_property id
var observedProps = []string{
	"pause",
	"volume",
	"duration",
	"path",
	"media-title",
	"metadata/by-key/artist",
	"metadata/by-key/album",
	"idle-active",
}

type PlayerStatus struct {
	Title    string
	Artist   string
	Album    string
	Current  float64
	Duration float64
	Paused   bool
	Volume   float64
	Path     string
}

const (
	volumeStep          = 5
	mpvSocketTimeout    = 5 * time.Second
	mpvSocketPollDelay  = 100 * time.Millisecond
	mpvDialProbeTimeout = 200 * time.Millisecond
)

func waitForMpvSocket(socketPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error

	for {
		conn, err := net.DialTimeout("unix", socketPath, mpvDialProbeTimeout)
		if err == nil {
			_ = conn.Close()
			return nil
		}

		lastErr = err
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for mpv IPC socket %s: %w", socketPath, lastErr)
		}

		time.Sleep(mpvSocketPollDelay)
	}
}

func stopMpvProcess() {
	if mpvCmd == nil || mpvCmd.Process == nil {
		return
	}

	_ = mpvCmd.Process.Signal(syscall.SIGTERM)

	done := make(chan error, 1)
	go func() {
		done <- mpvCmd.Wait()
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		_ = mpvCmd.Process.Kill()
		<-done
	}
}

func InitPlayer() error {
	socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("subtui_mpv_socket_%d", os.Getuid()))
	log.Printf("[Player] Initializing MPV IPC at %s", socketPath)

	// Clean up stale socket if one exists from a previous crash.
	_ = os.Remove(socketPath)

	killArg := fmt.Sprintf("--input-ipc-server=%s", socketPath)
	_ = exec.Command("pkill", "-f", "--", killArg).Run()

	replayGain := strings.ToLower(api.AppConfig.App.ReplayGain)
	if replayGain != "track" && replayGain != "album" {
		replayGain = "no"
	}

	gaplessPlayback := strings.ToLower(api.AppConfig.App.GaplessPlayBack)
	if gaplessPlayback != "no" && gaplessPlayback != "weak" {
		gaplessPlayback = "yes"
	}

	args := []string{
		"--idle",
		"--no-video",
		"--input-ipc-server=" + socketPath,
		"--gapless-audio=" + gaplessPlayback,
		"--prefetch-playlist=yes",
		"--replaygain=" + replayGain,
		"--loop-file=no",
		// mpv's default demuxer cache (150MiB ahead + 50MiB back) is sized for
		// video; 16MiB is still over a minute of FLAC
		"--demuxer-max-bytes=16MiB",
		"--demuxer-max-back-bytes=4MiB",
	}

	mpvCmd = exec.Command("mpv", args...)
	if err := mpvCmd.Start(); err != nil {
		return fmt.Errorf("failed to start mpv: %v", err)
	}

	if err := waitForMpvSocket(socketPath, mpvSocketTimeout); err != nil {
		stopMpvProcess()
		return err
	}

	client, err := dialIPC(socketPath, handleEvent)
	if err != nil {
		stopMpvProcess()
		return fmt.Errorf("failed to connect to mpv IPC: %w", err)
	}
	mpvClient = client

	for i, name := range observedProps {
		if _, err := client.exec("observe_property", i+1, name); err != nil {
			log.Printf("[Player] observe_property %s failed: %v", name, err)
		}
	}

	log.Printf("[Player] MPV started successfully")
	return nil
}

func ShutdownPlayer() {
	if mpvClient != nil {
		mpvClient.close()
	}
	stopMpvProcess()
	mpvClient = nil
	mpvCmd = nil
}

func PlaySong(songID string, startPaused bool) error {
	log.Printf("[Player] PlaySong called for ID: %s (Paused: %v)", songID, startPaused)

	if mpvClient == nil {
		return fmt.Errorf("player not initialized")
	}

	url := api.SubsonicStream(songID) + fmt.Sprintf("&_nonce=%d", time.Now().UnixNano())
	if _, err := mpvClient.exec("loadfile", url, "replace"); err != nil {
		return err
	}

	_ = mpvClient.setProperty("pause", startPaused)

	return nil
}

func EnqueueSong(songID string) error {
	if mpvClient == nil {
		return fmt.Errorf("player not initialized")
	}

	url := api.SubsonicStream(songID) + fmt.Sprintf("&_nonce=%d", time.Now().UnixNano())
	_, err := mpvClient.exec("loadfile", url, "append")
	return err
}

func UpdateNextSong(songID string) {
	if mpvClient == nil {
		return
	}

	_, _ = mpvClient.exec("playlist-clear")

	if songID != "" {
		_ = EnqueueSong(songID)
	}
}

func SetPause(paused bool) {
	if mpvClient == nil {
		return
	}
	_ = mpvClient.setProperty("pause", paused)
}

func TogglePause() {
	if mpvClient == nil {
		return
	}

	_, _ = mpvClient.exec("cycle", "pause")
}

func Stop() {
	if mpvClient == nil {
		return
	}

	_, _ = mpvClient.exec("stop")
}

func RestartSong() {
	if mpvClient == nil {
		return
	}

	_, _ = mpvClient.exec("seek", 0, "absolute")
}

func Back10Seconds() {
	if mpvClient == nil {
		return
	}

	_, _ = mpvClient.exec("seek", -10)
}

func Forward10Seconds() {
	if mpvClient == nil {
		return
	}

	_, _ = mpvClient.exec("seek", 10)
}

func SeekTo(seconds float64) {
	if mpvClient == nil {
		return
	}
	_ = mpvClient.setProperty("time-pos", seconds)
}

func VolumeUp() {
	if mpvClient == nil {
		return
	}

	newVolume := ((currentVolume() / volumeStep) + 1) * volumeStep
	SetVolume(newVolume)
}

func VolumeDown() {
	if mpvClient == nil {
		return
	}

	newVolume := ((currentVolume() - 1) / volumeStep) * volumeStep
	SetVolume(newVolume)
}

func GetVolume() float64 {
	if mpvClient == nil {
		return 0
	}

	return mpvClient.floatProperty("volume")
}

func currentVolume() int {
	stateMu.Lock()
	defer stateMu.Unlock()
	return int(state.volume)
}

func SetVolume(volume int) {
	if mpvClient == nil {
		return
	}

	if volume < 0 {
		volume = 0
	} else if volume > 100 {
		volume = 100
	}
	_ = mpvClient.setProperty("volume", volume)
}

// Changes is signalled (coalesced) whenever mpv reports a settled state
// change: a file finished loading, a seek completed, pause/volume/metadata
// changed, or playback went idle. Position updates are not signalled.
func Changes() <-chan struct{} {
	return changes
}

func notifyChange() {
	select {
	case changes <- struct{}{}:
	default:
	}
}

func asString(data any) string {
	if data == nil {
		return nilString
	}
	if s, ok := data.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", data)
}

// handleEvent runs on the IPC read loop
func handleEvent(msg ipcMessage) {
	switch msg.Event {
	case "property-change":
		stateMu.Lock()
		switch msg.Name {
		case "pause":
			state.paused, _ = msg.Data.(bool)
		case "volume":
			state.volume, _ = msg.Data.(float64)
		case "duration":
			state.duration, _ = msg.Data.(float64)
		case "path":
			state.path = asString(msg.Data)
			if msg.Data != nil {
				state.lastPath = state.path
			}
		case "media-title":
			state.title = asString(msg.Data)
		case "metadata/by-key/artist":
			state.artist = asString(msg.Data)
		case "metadata/by-key/album":
			state.album = asString(msg.Data)
		case "idle-active":
			state.idle, _ = msg.Data.(bool)
			if state.idle {
				state.lastPath = ""
			}
		}
		stateMu.Unlock()

		// path/duration/title flicker to nil between gapless files; the
		// file-loaded event that follows is the settled point
		switch msg.Name {
		case "pause", "volume", "idle-active", "metadata/by-key/artist", "metadata/by-key/album":
			notifyChange()
		}

	case "file-loaded", "playback-restart":
		notifyChange()
	}
}

func GetPlayerStatus() PlayerStatus {
	if mpvClient == nil {
		return PlayerStatus{}
	}

	stateMu.Lock()
	st := state
	stateMu.Unlock()

	// Between gapless files mpv briefly reports no path; only treat that
	// as stopped once mpv is actually idle
	path := st.path
	if path == nilString && !st.idle && st.lastPath != "" {
		path = st.lastPath
	}

	var pos float64
	if !st.idle {
		pos = mpvClient.floatProperty("time-pos")
	}

	return PlayerStatus{
		Title:    st.title,
		Artist:   st.artist,
		Album:    st.album,
		Current:  pos,
		Duration: st.duration,
		Paused:   st.paused,
		Volume:   st.volume,
		Path:     path,
	}
}
