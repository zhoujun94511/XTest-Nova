package touchreader

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
)

const (
	evSyn       = 0x00
	evAbs       = 0x03
	synReport   = 0x00
	absSlot     = 0x2f
	absX        = 0x35
	absY        = 0x36
	absTracking = 0x39
)

type Event struct {
	Operation string  `json:"operation"`
	Index     int     `json:"index"`
	XP        float64 `json:"xP"`
	YP        float64 `json:"yP"`
}
type inputEvent struct {
	kind, code uint16
	value      int32
}
type contact struct{ state, x, y int }
type Device struct {
	Path                 string
	MaxX, MaxY, Contacts int
}

// OSCapture exposes the same verified Protocol-B reader used by the WebSocket
// endpoint to higher-level recording services.
type OSCapture struct{}

func (OSCapture) Capture(ctx context.Context, sink func(time.Time, Event)) error {
	probeCtx, probeCancel := context.WithTimeout(ctx, 5*time.Second)
	probe, err := exec.CommandContext(probeCtx, "getevent", "-pl").Output()
	probeCancel()
	if err != nil {
		return fmt.Errorf("touch device probe: %w", err)
	}
	device, err := ParseDevice(string(probe))
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, "getevent", "-lt", device.Path)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	if err = command.Start(); err != nil {
		return err
	}
	readerState := state{maxX: device.MaxX, maxY: device.MaxY, maxContacts: device.Contacts}
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		input, ok := parseLine(scanner.Text())
		if !ok {
			continue
		}
		for _, event := range readerState.apply(input) {
			sink(time.Now().UTC(), event)
		}
	}
	if scanErr := scanner.Err(); scanErr != nil {
		_ = command.Wait()
		return scanErr
	}
	err = command.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

var deviceLine = regexp.MustCompile(`(?m)^add device \d+: (/dev/input/event\d+)`)
var maxLine = regexp.MustCompile(`ABS_MT_POSITION_([XY]).*max\s+([0-9A-Fa-fx]+)`)
var slotLine = regexp.MustCompile(`ABS_MT_SLOT.*max\s+([0-9A-Fa-fx]+)`)

func parseNumber(value string) (int, error) {
	base := 10
	if strings.HasPrefix(value, "0x") {
		base = 0
	}
	parsed, err := strconv.ParseInt(value, base, 32)
	return int(parsed), err
}
func ParseDevice(output string) (Device, error) {
	blocks := strings.Split(output, "add device ")
	for _, raw := range blocks {
		block := "add device " + raw
		pathMatch := deviceLine.FindStringSubmatch(block)
		if len(pathMatch) != 2 || !strings.Contains(block, "INPUT_PROP_DIRECT") {
			continue
		}
		device := Device{Path: pathMatch[1], Contacts: 10}
		for _, match := range maxLine.FindAllStringSubmatch(block, -1) {
			value, _ := parseNumber(match[2])
			if match[1] == "X" {
				device.MaxX = value
			} else {
				device.MaxY = value
			}
		}
		if match := slotLine.FindStringSubmatch(block); len(match) == 2 {
			value, _ := parseNumber(match[1])
			device.Contacts = value + 1
		} else {
			continue
		}
		if device.MaxX > 0 && device.MaxY > 0 {
			if device.Contacts > 10 {
				device.Contacts = 10
			}
			return device, nil
		}
	}
	return Device{}, errors.New("protocol-B touch device not found")
}
func parseLine(line string) (inputEvent, bool) {
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return inputEvent{}, false
	}
	kindName, codeName, valueText := fields[len(fields)-3], fields[len(fields)-2], fields[len(fields)-1]
	event := inputEvent{}
	switch kindName {
	case "EV_SYN":
		event.kind = evSyn
	case "EV_ABS":
		event.kind = evAbs
	default:
		return inputEvent{}, false
	}
	switch codeName {
	case "SYN_REPORT":
		event.code = synReport
	case "ABS_MT_SLOT":
		event.code = absSlot
	case "ABS_MT_POSITION_X":
		event.code = absX
	case "ABS_MT_POSITION_Y":
		event.code = absY
	case "ABS_MT_TRACKING_ID":
		event.code = absTracking
	default:
		return inputEvent{}, false
	}
	value, err := strconv.ParseUint(strings.TrimPrefix(valueText, "0x"), 16, 32)
	if err != nil {
		return inputEvent{}, false
	}
	event.value = int32(uint32(value))
	return event, true
}

type state struct {
	contacts                      [10]contact
	slot, maxX, maxY, maxContacts int
}

func (s *state) apply(event inputEvent) []Event {
	if event.kind == evAbs {
		switch event.code {
		case absSlot:
			s.slot = int(event.value)
			if s.slot < 0 || s.slot >= s.maxContacts {
				s.slot = 0
			}
		case absTracking:
			if event.value < 0 {
				s.contacts[s.slot].state = 3
			} else {
				s.contacts[s.slot].state = 1
			}
		case absX:
			s.contacts[s.slot].x = int(event.value)
		case absY:
			s.contacts[s.slot].y = int(event.value)
		}
		return nil
	}
	if event.kind != evSyn || event.code != synReport {
		return nil
	}
	result := make([]Event, 0)
	for index := 0; index < s.maxContacts; index++ {
		item := &s.contacts[index]
		if item.state == 0 {
			continue
		}
		operation := map[int]string{1: "d", 2: "m", 3: "u"}[item.state]
		result = append(result, Event{Operation: operation, Index: index, XP: float64(item.x) / float64(s.maxX), YP: float64(item.y) / float64(s.maxY)})
		if item.state == 3 {
			*item = contact{}
		} else {
			item.state = 2
		}
	}
	return result
}
func ServeWebSocket(w http.ResponseWriter, r *http.Request) {
	upgrade := r
	if r.Method != http.MethodGet {
		upgrade = r.Clone(r.Context())
		upgrade.Method = http.MethodGet
	}
	connection, err := websocket.Accept(w, upgrade, nil)
	if err != nil {
		return
	}
	defer func() { _ = connection.CloseNow() }()
	connection.SetReadLimit(4096)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go func() {
		for {
			if _, _, readErr := connection.Read(ctx); readErr != nil {
				cancel()
				return
			}
		}
	}()
	probeCtx, probeCancel := context.WithTimeout(ctx, 5*time.Second)
	probe, err := exec.CommandContext(probeCtx, "getevent", "-pl").Output()
	probeCancel()
	if err != nil {
		_ = connection.Close(websocket.StatusInternalError, "touch device probe failed")
		return
	}
	device, err := ParseDevice(string(probe))
	if err != nil {
		_ = connection.Close(websocket.StatusUnsupportedData, err.Error())
		return
	}
	command := exec.CommandContext(ctx, "getevent", "-lt", device.Path)
	stdout, err := command.StdoutPipe()
	if err != nil {
		_ = connection.Close(websocket.StatusInternalError, "touch reader unavailable")
		return
	}
	if err = command.Start(); err != nil {
		_ = connection.Close(websocket.StatusInternalError, "touch reader unavailable")
		return
	}
	defer func() {
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		_ = command.Wait()
	}()
	readerState := state{maxX: device.MaxX, maxY: device.MaxY, maxContacts: device.Contacts}
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		input, ok := parseLine(scanner.Text())
		if !ok {
			continue
		}
		for _, event := range readerState.apply(input) {
			payload, _ := json.Marshal(event)
			writeCtx, done := context.WithTimeout(ctx, 2*time.Second)
			err = connection.Write(writeCtx, websocket.MessageText, payload)
			done()
			if err != nil {
				return
			}
		}
	}
	if err = scanner.Err(); err != nil {
		_ = connection.Close(websocket.StatusInternalError, fmt.Sprintf("touch stream: %v", err))
	}
}
