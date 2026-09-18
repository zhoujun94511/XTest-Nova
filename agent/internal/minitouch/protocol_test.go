package minitouch

import (
	"bufio"
	"strings"
	"testing"
)

func TestReadBannerAndTouchCommand(t *testing.T) {
	contacts, x, y, p, err := readBanner(bufio.NewReader(strings.NewReader("v 1\n^ 10 1080 2400 255\n$ 42\n")))
	if err != nil || contacts != 10 || x != 1080 || y != 2400 || p != 255 {
		t.Fatalf("banner=%d,%d,%d,%d err=%v", contacts, x, y, p, err)
	}
	command, err := touchCommand(TouchRequest{Operation: "d", Index: 0, PercentX: .5, PercentY: .25, Pressure: .5}, contacts, x, y, p)
	if err != nil || command != "d 0 540 600 127\n" {
		t.Fatalf("command=%q err=%v", command, err)
	}
}
func TestTouchCommandRejectsUnsafeValues(t *testing.T) {
	if _, err := touchCommand(TouchRequest{Operation: "d", PercentX: 2}, 1, 100, 100, 255); err == nil {
		t.Fatal("invalid coordinate accepted")
	}
}
