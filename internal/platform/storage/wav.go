package storage

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// GenerateDemoWAV writes a short, listenable demo track so the product can be
// exercised end-to-end without the Python model service.
func GenerateDemoWAV(path string, seconds int, seed string) error {
	if seconds <= 0 {
		seconds = 30
	}
	if seconds > 12 {
		seconds = 12
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	const sampleRate = 44100
	total := seconds * sampleRate
	data := make([]byte, total*2)

	// Derive a stable root note from the seed.
	var h uint32
	for _, b := range []byte(seed) {
		h = h*31 + uint32(b)
	}
	roots := []float64{220.00, 246.94, 261.63, 293.66, 329.63}
	root := roots[int(h)%len(roots)]
	ratios := []float64{1, 1.25, 1.5}

	for i := 0; i < total; i++ {
		t := float64(i) / sampleRate
		bar := int(t/2) % 4
		freq := root
		if bar == 1 {
			freq = root * 1.25
		} else if bar == 2 {
			freq = root * 1.5
		} else if bar == 3 {
			freq = root * 1.125
		}

		env := 1.0
		local := math.Mod(t, 2.0)
		if local < 0.05 {
			env = local / 0.05
		} else if local > 1.85 {
			env = (2.0 - local) / 0.15
		}

		var v float64
		for _, r := range ratios {
			v += math.Sin(2*math.Pi*freq*r*t) / float64(len(ratios))
		}
		v += 0.18 * math.Sin(2*math.Pi*55*t)
		v *= env * 0.32

		s := int16(math.Max(-1, math.Min(1, v)) * 32767)
		binary.LittleEndian.PutUint16(data[i*2:], uint16(s))
	}

	header := make([]byte, 44)
	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], uint32(36+len(data)))
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1)
	binary.LittleEndian.PutUint16(header[22:24], 1)
	binary.LittleEndian.PutUint32(header[24:28], sampleRate)
	binary.LittleEndian.PutUint32(header[28:32], sampleRate*2)
	binary.LittleEndian.PutUint16(header[32:34], 2)
	binary.LittleEndian.PutUint16(header[34:36], 16)
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], uint32(len(data)))

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(header); err != nil {
		return err
	}
	_, err = f.Write(data)
	return err
}

func JSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func MustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte(fmt.Sprintf("%q", err.Error()))
	}
	return b
}
