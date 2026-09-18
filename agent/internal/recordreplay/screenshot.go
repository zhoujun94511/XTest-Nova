package recordreplay

import (
	"encoding/hex"
	"errors"
	"image"
	_ "image/png"
	"io"
	"math/bits"
)

func screenshotHash(reader io.Reader) (string, error) {
	value, _, err := image.Decode(reader)
	if err != nil {
		return "", err
	}
	bounds := value.Bounds()
	if bounds.Dx() < 8 || bounds.Dy() < 8 {
		return "", errors.New("screenshot is too small")
	}
	levels := [64]uint32{}
	var total uint64
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			px, py := bounds.Min.X+(2*x+1)*bounds.Dx()/16, bounds.Min.Y+(2*y+1)*bounds.Dy()/16
			r, g, b, _ := value.At(px, py).RGBA()
			level := (299*r + 587*g + 114*b) / 1000
			levels[y*8+x], total = level, total+uint64(level)
		}
	}
	average := uint32(total / 64)
	var hash uint64
	for index, level := range levels {
		if level >= average {
			hash |= 1 << index
		}
	}
	encoded := make([]byte, 8)
	for index := range encoded {
		encoded[7-index] = byte(hash >> (8 * index))
	}
	return hex.EncodeToString(encoded), nil
}

func hashDistance(left, right string) (int, error) {
	a, err := hex.DecodeString(left)
	if err != nil || len(a) != 8 {
		return 0, errors.New("invalid screenshot hash")
	}
	b, err := hex.DecodeString(right)
	if err != nil || len(b) != 8 {
		return 0, errors.New("invalid screenshot hash")
	}
	distance := 0
	for index := range a {
		distance += bits.OnesCount8(a[index] ^ b[index])
	}
	return distance, nil
}
