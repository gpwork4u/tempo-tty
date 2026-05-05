package audio

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/flac"
	"github.com/gopxl/beep/v2/mp3"
	"github.com/gopxl/beep/v2/vorbis"
	"github.com/gopxl/beep/v2/wav"
)

// Decode opens an audio file and returns a streamer + format.
// Caller must Close the streamer when done.
func Decode(path string) (beep.StreamSeekCloser, beep.Format, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, beep.Format{}, err
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp3":
		return mp3.Decode(f)
	case ".wav":
		return wav.Decode(f)
	case ".ogg":
		return vorbis.Decode(f)
	case ".flac":
		return flac.Decode(f)
	}
	f.Close()
	return nil, beep.Format{}, fmt.Errorf("unsupported audio extension: %s", filepath.Ext(path))
}

// LoadMono drains the streamer into a mono float64 slice.
func LoadMono(path string) ([]float64, int, error) {
	s, format, err := Decode(path)
	if err != nil {
		return nil, 0, err
	}
	defer s.Close()

	hint := s.Len()
	if hint <= 0 {
		hint = 1024 * 1024
	}
	out := make([]float64, 0, hint)
	buf := make([][2]float64, 4096)
	for {
		n, ok := s.Stream(buf)
		for i := 0; i < n; i++ {
			out = append(out, (buf[i][0]+buf[i][1])*0.5)
		}
		if !ok {
			break
		}
	}
	return out, int(format.SampleRate), nil
}

// Counter wraps a streamer and atomically tracks samples consumed,
// providing an audio clock independent of wall time.
type Counter struct {
	S          beep.Streamer
	SampleRate int
	n          atomic.Int64
}

func (c *Counter) Stream(samples [][2]float64) (int, bool) {
	n, ok := c.S.Stream(samples)
	c.n.Add(int64(n))
	return n, ok
}

func (c *Counter) Err() error { return c.S.Err() }

// Position returns seconds of audio consumed.
func (c *Counter) Position() float64 {
	return float64(c.n.Load()) / float64(c.SampleRate)
}
