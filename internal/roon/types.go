package roon

type CoreID string
type ZoneID string
type ImageKey string

type Core struct {
	ID   CoreID
	Name string

	Host string
	Port int
}

type Zone struct {
	ID   ZoneID
	Name string

	NowPlaying *NowPlaying
}

type NowPlaying struct {
	Title  string
	Artist string
	Album  string

	ImageKey ImageKey
}
