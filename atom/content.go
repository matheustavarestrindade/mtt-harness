package atom

type MediaType string

const (
	Text  MediaType = "text"
	Image MediaType = "image"
	Audio MediaType = "audio"
	File  MediaType = "file"
)

type Content struct {
	Type     MediaType
	Text     string
	Data     []byte
	MIME     string
	URL      string
	Filename string
	AudioID  string
}
