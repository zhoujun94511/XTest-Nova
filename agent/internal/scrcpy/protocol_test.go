package scrcpy

import (
	"encoding/binary"
	"strings"
	"testing"
)

func TestEmbeddedServerDigest(t *testing.T) {
	if got := digest(serverJar); got != ServerSHA256 {
		t.Fatalf("server digest = %s", got)
	}
}

func TestOptions(t *testing.T) {
	low, err := optionsFor("low")
	if err != nil || low.FPS != 10 || low.MaxSize != 480 || low.BitRate != 1_000_000 {
		t.Fatalf("low options = %#v, %v", low, err)
	}
	original, err := optionsFor("original")
	if err != nil || original.FPS != 0 || original.MaxSize != 0 {
		t.Fatalf("original options = %#v, %v", original, err)
	}
	if _, err := optionsFor("unknown"); err == nil {
		t.Fatal("unknown definition accepted")
	}
}

func TestServerArgumentsKeepVerifiedArtifact(t *testing.T) {
	arguments := strings.Join(serverArgs(Options{FPS: 10, MaxSize: 480, BitRate: 1_000_000, StayAwake: true}, 42), " ")
	for _, expected := range []string{"scid=0000002a", "video_codec=h264", "raw_stream=true", "cleanup=false", "max_fps=10", "max_size=480"} {
		if !strings.Contains(arguments, expected) {
			t.Fatalf("arguments missing %q: %s", expected, arguments)
		}
	}
}

func TestControlOnlyServerDisablesVideo(t *testing.T) {
	arguments := strings.Join(serverArgsFor(Options{}, 7, false), " ")
	if !strings.Contains(arguments, "video=false") || !strings.Contains(arguments, "control=true") {
		t.Fatalf("control arguments = %s", arguments)
	}
}

func TestParsePhysicalSizePrefersOverride(t *testing.T) {
	width, height, ok := parsePhysicalSize("Physical size: 1156x2510\nOverride size: 588x1280")
	if !ok || width != 588 || height != 1280 {
		t.Fatalf("size = %dx%d, ok=%v", width, height, ok)
	}
}

func TestTouchFrameUsesEncodedDimensions(t *testing.T) {
	frame, err := (Event{Type: 0, Operation: "d", Index: 1, PercentX: .5, PercentY: .25, ScreenWidth: 588, ScreenHeight: 1280}).frame(1156, 2510)
	if err != nil {
		t.Fatal(err)
	}
	if len(frame) != 32 || frame[0] != 2 || frame[1] != 0 {
		t.Fatalf("unexpected header: %v", frame[:2])
	}
	if x := binary.BigEndian.Uint32(frame[10:14]); x != 294 {
		t.Fatalf("x = %d", x)
	}
	if y := binary.BigEndian.Uint32(frame[14:18]); y != 320 {
		t.Fatalf("y = %d", y)
	}
	if width := binary.BigEndian.Uint16(frame[18:20]); width != 588 {
		t.Fatalf("width = %d", width)
	}
}

func TestControlFramesAndValidation(t *testing.T) {
	keys, err := (Event{Type: 1, Keycode: 3}).frame(1080, 1920)
	if err != nil || len(keys) != 28 || keys[0] != 0 || keys[14] != 0 {
		t.Fatalf("key frames invalid: %d, %v", len(keys), err)
	}
	clip, err := (Event{Type: 5, Sequence: 42, Text: "Nova", Paste: true}).frame(1080, 1920)
	if err != nil || clip[0] != 9 || binary.BigEndian.Uint64(clip[1:9]) != 42 || clip[9] != 1 {
		t.Fatalf("clipboard frame invalid: %v", err)
	}
	unicodeText, err := (Event{Type: 2, Text: "你好"}).frame(1080, 1920)
	if err != nil || unicodeText[0] != 9 || unicodeText[9] != 1 {
		t.Fatalf("unicode text must use clipboard paste: %v", err)
	}
	if _, err := (Event{Type: 0, Operation: "d", PercentX: 1.1, PercentY: 0}).frame(1080, 1920); err == nil {
		t.Fatal("out-of-range coordinates accepted")
	}
	if _, err := (Event{Type: 99}).frame(1080, 1920); err == nil {
		t.Fatal("unsupported event accepted")
	}
}
