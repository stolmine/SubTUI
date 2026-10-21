package ui

import (
	"image"
	"time"

	"github.com/MattiaPun/SubTUI/v2/internal/api"
	"github.com/MattiaPun/SubTUI/v2/internal/integration"
	"github.com/MattiaPun/SubTUI/v2/internal/player"
	"github.com/charmbracelet/bubbles/textinput"
)

var albumTypes = []string{"All", "Random", "Favorites", "Recently Added", "Recently Played", "Most Played"}

// --- MODEL ---
type model struct {
	textInput    textinput.Model
	songs        []api.Song
	songsPrev    []api.Song
	albums       []api.Album
	albumsPrev   []api.Album
	artists      []api.Artist
	artistsPrev  []api.Artist
	playlists    []api.Playlist
	playerStatus player.PlayerStatus

	// Navigation State
	focus          int
	cursorMain     int
	cursorMainPrev int
	cursorSide     int
	sideOffset     int
	cursorPopup    int
	mainOffset     int
	mainOffsetPrev int

	// Window Dimensions
	width  int
	height int

	// View Mode
	viewMode        int
	filterMode      int
	displayMode     int
	displayModePrev int

	// Cover Art
	coverArt         image.Image
	coverArtRendered string // mosaic render of coverArt, cached until the image or size changes

	// App State
	err                error
	loading            bool
	lastPlayedSongPath string
	scrobbled          bool
	loginErr           string
	windowTitle        string
	positionTickGen    int
	discordRPC         bool
	notify             bool

	// Rendered header and center panes, shared by all copies of the model
	paneCache *paneCache
	artCache  *artCache

	// AutoDJ request state (enabled and contrast live in api.AppConfig.AutoDJ)
	autoDJFetching bool
	autoDJFailed   bool

	// Integrations
	dbusInstance    *integration.Instance
	discordInstance *integration.DiscordInstance

	// Queue System
	queue      []api.Song
	queueIndex int
	loopMode   int

	// Stars
	starredMap map[string]bool

	// Login State
	loginInputs []textinput.Model
	loginFocus  int
	loginType   int

	// Input State
	lastKey string

	// View States
	showMediaPlayer bool
	showHelp        bool
	showPlaylists   bool
	confirmQuit     bool
	helpModel       HelpModel

	// Pagination State
	lastSearchQuery string
	albumListType   string
	pageOffset      int
	pageHasMore     bool

	// Song sort mode (in a playlist)
	songSortBy  int
	songSortAsc bool

	// Selection state
	showSelection   bool
	selectionAnchor int
	selectionMap    map[int]bool

	// Mouse state
	lastClickTime time.Time
	lastClickId   string

	// Lyrics
	songLyrics      []api.StructuredLyrics
	songLinesOffset int
}

type HelpModel struct {
	Width  int
	Height int
}

type ContentModel struct {
	Content string
}

type BackgroundWrapper struct {
	RenderedView string
}

type loginResultMsg struct {
	err error
}

type songsResultMsg struct {
	songs []api.Song
}

type albumsResultMsg struct {
	albums []api.Album
}

type artistsResultMsg struct {
	artists []api.Artist
}

type playlistResultMsg struct {
	playlists []api.Playlist
}

// randomAlbumMsg carries the tracks of a random album to play in order
type randomAlbumMsg struct {
	songs []api.Song
}

type shuffledSongsMsg struct {
	songs      []api.Song
	updateView bool
}

type starredResultMsg struct {
	result *api.SearchResult3
}

type playQueueResultMsg struct {
	result *api.PlayQueue
}

type viewStarredSongsMsg *api.SearchResult3

type coverArtMsg struct {
	img image.Image
}

type createShareMsg struct {
	url string
}

type getLyricsMsg struct {
	result []api.StructuredLyrics
}

type errMsg struct {
	err error
}

type statusMsg player.PlayerStatus

type paneCache struct {
	valid       bool
	top, center string
}

// playerChangeMsg carries the player status after mpv reported a change
type playerChangeMsg player.PlayerStatus

type positionTickMsg struct {
	gen    int
	status player.PlayerStatus
}

type SetDBusMsg struct {
	Instance *integration.Instance
}

type SetDiscordMsg struct {
	Instance *integration.DiscordInstance
}
