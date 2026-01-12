package roon

type CoreID string
type ZoneID string
type ImageKey string

type ZoneState string

const (
	ZoneStatePlaying ZoneState = "playing"
	ZoneStatePaused  ZoneState = "paused"
	ZoneStateLoading ZoneState = "loading"
	ZoneStateStopped ZoneState = "stopped"
)

type Core struct {
	ID   CoreID
	Name string

	Host string
	Port int
}

type Zone struct {
	ID   ZoneID
	Name string

	State      ZoneState
	NowPlaying *NowPlaying
}

type NowPlaying struct {
	Title  string
	Artist string
	Album  string

	ImageKey ImageKey
}
