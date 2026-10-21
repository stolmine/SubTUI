//go:build linux || freebsd

package integration

import (
	"log"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"
)

// A position jump larger than this (vs. the extrapolated position) is
// reported to MPRIS clients as a seek.
const seekThreshold = 1500 * time.Millisecond

type Instance struct {
	props *prop.Properties
	conn  *dbus.Conn

	// Last values pushed to D-Bus, so unchanged values are not re-broadcast
	lastStatus     string
	lastPosition   int64
	lastPositionAt time.Time
	cleared        bool
}

func Init(p *tea.Program) *Instance {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil
	}

	ins := &Instance{conn: conn}

	mp2 := &MediaPlayer2{Program: p}
	err = conn.Export(mp2, "/org/mpris/MediaPlayer2", "org.mpris.MediaPlayer2.Player")
	if err != nil {
		log.Printf("MPRIS Export Error: %v", err)
		return nil
	}

	err = conn.Export(mp2, "/org/mpris/MediaPlayer2", "org.mpris.MediaPlayer2")
	if err != nil {
		log.Printf("MPRIS Root Export Error: %v", err)
	}

	ins.props, _ = prop.Export(
		conn,
		"/org/mpris/MediaPlayer2",
		map[string]map[string]*prop.Prop{
			"org.mpris.MediaPlayer2":        rootProps,
			"org.mpris.MediaPlayer2.Player": playerProps,
		},
	)

	reply, err := conn.RequestName("org.mpris.MediaPlayer2.subtui", dbus.NameFlagReplaceExisting)
	if err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		log.Printf("MPRIS Name Error: %v", err)
	}

	return ins
}

func (ins *Instance) GetStatus() string {
	if ins == nil || ins.props == nil {
		return "Stopped"
	}

	variant, err := ins.props.Get("org.mpris.MediaPlayer2.Player", "PlaybackStatus")
	if err != nil {
		return "Stopped"
	}

	if s, ok := variant.Value().(string); ok {
		return s
	}

	return "Stopped"
}

func (ins *Instance) UpdateStatus(status string) {
	if ins == nil || status == ins.lastStatus {
		return
	}

	ins.lastStatus = status
	_ = ins.props.Set("org.mpris.MediaPlayer2.Player", "PlaybackStatus", dbus.MakeVariant(string(status)))
}

// UpdatePosition stores the position (Position does not emit) and sends a
// Seeked signal when it jumps away from where a client would extrapolate it.
func (ins *Instance) UpdatePosition(position int64) {
	if ins == nil {
		return
	}

	expected := ins.lastPosition
	if ins.lastStatus == "Playing" {
		expected += time.Since(ins.lastPositionAt).Microseconds()
	}

	drift := position - expected
	if drift < 0 {
		drift = -drift
	}

	seeked := !ins.lastPositionAt.IsZero() && drift > seekThreshold.Microseconds()

	ins.lastPosition = position
	ins.lastPositionAt = time.Now()
	_ = ins.props.Set("org.mpris.MediaPlayer2.Player", "Position", dbus.MakeVariant(position))

	if seeked {
		_ = ins.conn.Emit("/org/mpris/MediaPlayer2", "org.mpris.MediaPlayer2.Player.Seeked", position)
	}
}

func (ins *Instance) UpdateMetadata(meta Metadata) {
	if ins == nil {
		return
	}

	ins.cleared = false
	_ = ins.props.Set("org.mpris.MediaPlayer2.Player", "Metadata", dbus.MakeVariant(meta.ToMap()))
}

func (ins *Instance) ClearMetadata() {
	if ins == nil || ins.cleared {
		return
	}
	ins.cleared = true

	emptyMeta := make(map[string]dbus.Variant)
	emptyMeta["mpris:trackid"] = dbus.MakeVariant(dbus.ObjectPath("/org/mpris/MediaPlayer2/TrackList/NoTrack"))

	_ = ins.props.Set("org.mpris.MediaPlayer2.Player", "Metadata", dbus.MakeVariant(emptyMeta))
	ins.UpdateStatus("Stopped")
}

func (ins *Instance) Close() {
	if ins != nil && ins.conn != nil {
		_ = ins.conn.Close()
	}
}
