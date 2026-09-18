package scrcpy

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"unicode/utf8"
)

const maxTextBytes = (1 << 18) - 14

type Event struct {
	Type         int     `json:"type"`
	Operation    string  `json:"operation"`
	Index        int     `json:"index"`
	PercentX     float64 `json:"xP"`
	PercentY     float64 `json:"yP"`
	Milliseconds int     `json:"milliseconds"`
	Keycode      int     `json:"keycode"`
	Text         string  `json:"text"`
	Paste        bool    `json:"paste"`
	Sequence     uint64  `json:"sequence"`
	Horizontal   float64 `json:"hScroll"`
	Vertical     float64 `json:"vScroll"`
	ScreenWidth  int     `json:"screenWidth"`
	ScreenHeight int     `json:"screenHeight"`
}

func (e Event) frame(fallbackWidth, fallbackHeight uint16) ([]byte, error) {
	if e.Milliseconds < 0 || e.Milliseconds > 60000 {
		return nil, errors.New("milliseconds must be between 0 and 60000")
	}
	switch e.Type {
	case 0:
		return e.touchFrame(fallbackWidth, fallbackHeight)
	case 1:
		return keyFrames(e.Keycode)
	case 2:
		return textFrame(e.Text)
	case 3:
		return []byte{11}, nil
	case 4:
		return []byte{8, 0}, nil
	case 5:
		return clipboardFrame(e.Sequence, e.Text, e.Paste)
	case 6:
		return e.scrollFrame(fallbackWidth, fallbackHeight)
	default:
		return nil, fmt.Errorf("unsupported scrcpy event type: %d", e.Type)
	}
}

func (e Event) dimensions(fallbackWidth, fallbackHeight uint16) (uint16, uint16, error) {
	width, height := fallbackWidth, fallbackHeight
	if e.ScreenWidth != 0 || e.ScreenHeight != 0 {
		if e.ScreenWidth < 1 || e.ScreenWidth > 0xffff || e.ScreenHeight < 1 || e.ScreenHeight > 0xffff {
			return 0, 0, errors.New("screen dimensions must both be between 1 and 65535")
		}
		width, height = uint16(e.ScreenWidth), uint16(e.ScreenHeight)
	}
	if width == 0 || height == 0 {
		return 0, 0, errors.New("screen dimensions unavailable")
	}
	return width, height, nil
}

func position(xPercent, yPercent float64, width, height uint16) (uint32, uint32, error) {
	if math.IsNaN(xPercent) || math.IsNaN(yPercent) || math.IsInf(xPercent, 0) || math.IsInf(yPercent, 0) {
		return 0, 0, errors.New("coordinates must be finite")
	}
	if xPercent < 0 || xPercent > 1 || yPercent < 0 || yPercent > 1 {
		return 0, 0, errors.New("coordinates must be between 0 and 1")
	}
	return uint32(xPercent * float64(width)), uint32(yPercent * float64(height)), nil
}

func (e Event) touchFrame(fallbackWidth, fallbackHeight uint16) ([]byte, error) {
	if e.Operation == "c" {
		return nil, nil
	}
	if e.Index < 0 || e.Index > 9 {
		return nil, errors.New("touch index must be between 0 and 9")
	}
	action := byte(0)
	switch e.Operation {
	case "d":
		action = 0
	case "u":
		action = 1
	case "m":
		action = 2
	default:
		return nil, fmt.Errorf("unsupported touch operation: %s", e.Operation)
	}
	width, height, err := e.dimensions(fallbackWidth, fallbackHeight)
	if err != nil {
		return nil, err
	}
	x, y, err := position(e.PercentX, e.PercentY, width, height)
	if err != nil {
		return nil, err
	}
	frame := make([]byte, 32)
	frame[0], frame[1] = 2, action
	binary.BigEndian.PutUint64(frame[2:10], uint64(e.Index))
	binary.BigEndian.PutUint32(frame[10:14], x)
	binary.BigEndian.PutUint32(frame[14:18], y)
	binary.BigEndian.PutUint16(frame[18:20], width)
	binary.BigEndian.PutUint16(frame[20:22], height)
	if action != 1 {
		binary.BigEndian.PutUint16(frame[22:24], 0xffff)
		frame[31] = 1
	}
	if action == 0 || action == 1 {
		frame[27] = 1
	}
	return frame, nil
}

func keyFrames(keycode int) ([]byte, error) {
	if keycode < 0 || keycode > 1000 {
		return nil, errors.New("keycode must be between 0 and 1000")
	}
	frames := make([]byte, 28)
	for i, action := range []byte{0, 1} {
		offset := i * 14
		frames[offset], frames[offset+1] = 0, action
		binary.BigEndian.PutUint32(frames[offset+2:offset+6], uint32(keycode))
	}
	return frames, nil
}

func textFrame(text string) ([]byte, error) {
	if !utf8.ValidString(text) {
		return nil, errors.New("text must be valid UTF-8")
	}
	if len(text) > maxTextBytes {
		return nil, errors.New("text is too large")
	}
	for _, character := range text {
		if character > 0x7f {
			return clipboardFrame(0, text, true)
		}
	}
	data := []byte(text)
	frame := make([]byte, 5+len(data))
	frame[0] = 1
	binary.BigEndian.PutUint32(frame[1:5], uint32(len(data)))
	copy(frame[5:], data)
	return frame, nil
}

func clipboardFrame(sequence uint64, text string, paste bool) ([]byte, error) {
	if !utf8.ValidString(text) {
		return nil, errors.New("clipboard text must be valid UTF-8")
	}
	if len(text) > maxTextBytes {
		return nil, errors.New("clipboard text is too large")
	}
	data := []byte(text)
	frame := make([]byte, 14+len(data))
	frame[0] = 9
	binary.BigEndian.PutUint64(frame[1:9], sequence)
	if paste {
		frame[9] = 1
	}
	binary.BigEndian.PutUint32(frame[10:14], uint32(len(data)))
	copy(frame[14:], data)
	return frame, nil
}

func fixedPoint(value float64) (uint16, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, errors.New("scroll distance must be finite")
	}
	value = math.Max(-16, math.Min(16, value)) / 16
	if value >= 0 {
		return uint16(value * 32767), nil
	}
	return uint16(int16(value * 32768)), nil
}

func (e Event) scrollFrame(fallbackWidth, fallbackHeight uint16) ([]byte, error) {
	width, height, err := e.dimensions(fallbackWidth, fallbackHeight)
	if err != nil {
		return nil, err
	}
	x, y, err := position(e.PercentX, e.PercentY, width, height)
	if err != nil {
		return nil, err
	}
	horizontal, err := fixedPoint(e.Horizontal)
	if err != nil {
		return nil, err
	}
	vertical, err := fixedPoint(e.Vertical)
	if err != nil {
		return nil, err
	}
	frame := make([]byte, 21)
	frame[0] = 3
	binary.BigEndian.PutUint32(frame[1:5], x)
	binary.BigEndian.PutUint32(frame[5:9], y)
	binary.BigEndian.PutUint16(frame[9:11], width)
	binary.BigEndian.PutUint16(frame[11:13], height)
	binary.BigEndian.PutUint16(frame[13:15], horizontal)
	binary.BigEndian.PutUint16(frame[15:17], vertical)
	binary.BigEndian.PutUint32(frame[17:21], 1)
	return frame, nil
}
