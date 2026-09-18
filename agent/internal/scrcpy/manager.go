package scrcpy

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

const (
	ServerVersion         = "4.1"
	ServerSHA256          = "deacb991ed2509715160ffdc7907e47b4160eb30d1566217e9047fd5b8850cae"
	defaultPath           = "/data/local/tmp/xtest-nova-scrcpy-server-v4.1.jar"
	connectWait           = 8 * time.Second
	maxMessage            = 1 << 20
	clipboardAckQueueSize = 16
)

var (
	errDeviceResponseTimeout = errors.New("scrcpy device response timed out")
	errControlSessionClosed  = errors.New("scrcpy control session closed")
)

// serverJar is the unmodified official Genymobile scrcpy-server v4.1 artifact.
//
//go:embed scrcpy-server-v4.1.jar
var serverJar []byte

type Options struct {
	FPS, MaxSize, BitRate int
	StayAwake             bool
}

func optionsFor(definition string) (Options, error) {
	options := Options{FPS: 30, MaxSize: 1280, BitRate: 8_000_000, StayAwake: true}
	switch definition {
	case "", "normal", "middle":
	case "high":
		options.MaxSize = 2048
	case "low":
		options.FPS, options.MaxSize, options.BitRate = 10, 480, 1_000_000
	case "original":
		options.FPS, options.MaxSize = 0, 0
	default:
		return Options{}, fmt.Errorf("unsupported scrcpy definition: %s", definition)
	}
	return options, nil
}

type Manager struct {
	path          string
	startMu       sync.Mutex
	mu            sync.RWMutex
	current       *session
	controlMu     sync.Mutex
	controlActive bool
	nextID        atomic.Uint32
	nextSequence  atomic.Uint64
}

func New() *Manager {
	m := &Manager{path: defaultPath}
	m.nextID.Store(uint32(time.Now().UnixNano()) & 0x7fffffff)
	return m
}

func (m *Manager) Close() {
	m.mu.Lock()
	current := m.current
	m.current = nil
	m.mu.Unlock()
	if current != nil {
		current.close()
	}
}

func (m *Manager) ServeWebSocket(w http.ResponseWriter, r *http.Request, channel, definition string) {
	connection, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		return
	}
	defer func() { _ = connection.CloseNow() }()
	connection.SetReadLimit(maxMessage)
	switch channel {
	case "screen":
		err = m.serveScreen(r.Context(), connection, definition)
	case "control":
		err = m.serveControl(r.Context(), connection)
	default:
		err = fmt.Errorf("unsupported scrcpy channel: %s", channel)
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		_ = writeJSON(r.Context(), connection, map[string]any{"success": false, "error": err.Error()})
	}
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (m *Manager) installServer() error {
	if got := digest(serverJar); got != ServerSHA256 {
		return fmt.Errorf("embedded scrcpy server digest mismatch: %s", got)
	}
	if current, err := os.ReadFile(m.path); err == nil && digest(current) == ServerSHA256 {
		return nil
	}
	temporary := m.path + ".tmp"
	if err := os.WriteFile(temporary, serverJar, 0644); err != nil {
		return fmt.Errorf("stage scrcpy server: %w", err)
	}
	if err := os.Rename(temporary, m.path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("activate scrcpy server: %w", err)
	}
	return nil
}

func serverArgs(options Options, id uint32) []string {
	return serverArgsFor(options, id, true)
}

func serverArgsFor(options Options, id uint32, video bool) []string {
	args := []string{"/", "com.genymobile.scrcpy.Server", ServerVersion,
		fmt.Sprintf("scid=%08x", id), "log_level=warn", "tunnel_forward=true",
		"video=" + strconv.FormatBool(video), "audio=false", "control=true", "video_codec=h264", "raw_stream=true",
		"stay_awake=" + strconv.FormatBool(options.StayAwake), "cleanup=false"}
	if options.MaxSize > 0 {
		args = append(args, fmt.Sprintf("max_size=%d", options.MaxSize))
	}
	if options.FPS > 0 {
		args = append(args, fmt.Sprintf("max_fps=%d", options.FPS))
	}
	if options.BitRate > 0 {
		args = append(args, fmt.Sprintf("video_bit_rate=%d", options.BitRate))
	}
	return args
}

func (m *Manager) startControlOnly(ctx context.Context) (*session, error) {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	if err := m.installServer(); err != nil {
		return nil, err
	}
	id := m.nextID.Add(1) & 0x7fffffff
	processCtx, cancel := context.WithCancel(context.Background())
	command := exec.CommandContext(processCtx, "app_process", serverArgsFor(Options{StayAwake: false}, id, false)...)
	command.Env = append(os.Environ(), "CLASSPATH="+m.path)
	command.Stdout, command.Stderr = log.Writer(), log.Writer()
	if err := command.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start scrcpy control: %w", err)
	}
	s := newSession(command, cancel)
	control, err := dialSocket(ctx, fmt.Sprintf("@scrcpy_%08x", id))
	if err != nil {
		s.close()
		_ = command.Wait()
		return nil, err
	}
	s.control = control
	go s.readDeviceMessages()
	go func() { _ = command.Wait(); s.close() }()
	return s, nil
}

// InjectEvents uses an isolated official scrcpy control session and never
// replaces a live screen-stream session.
func (m *Manager) InjectEvents(ctx context.Context, events []Event) error {
	if len(events) == 0 || len(events) > 1000 {
		return errors.New("scrcpy control batch must contain 1-1000 events")
	}
	s, err := m.startControlOnly(ctx)
	if err != nil {
		return err
	}
	defer s.close()
	width, height := physicalSize(ctx)
	for _, event := range events {
		if event.Milliseconds > 0 {
			timer := time.NewTimer(time.Duration(event.Milliseconds) * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
		frame, frameErr := event.frame(width, height)
		if frameErr != nil {
			return frameErr
		}
		if len(frame) > 0 {
			if err = s.write(frame); err != nil {
				return err
			}
		}
		if event.Type == 5 && event.Sequence != 0 {
			if _, err = s.waitClipboardAck(ctx, 2*time.Second, event.Sequence); err != nil {
				if errors.Is(err, errDeviceResponseTimeout) {
					return errors.New("scrcpy clipboard acknowledgement timed out")
				}
				return err
			}
		}
	}
	return nil
}

func (m *Manager) InjectText(ctx context.Context, value string) error {
	sequence := m.nextSequence.Add(1)
	return m.InjectEvents(ctx, []Event{{Type: 5, Text: value, Paste: true, Sequence: sequence}})
}

func dialSocket(ctx context.Context, name string) (net.Conn, error) {
	deadline := time.Now().Add(connectWait)
	var last error
	for time.Now().Before(deadline) {
		dialer := net.Dialer{Timeout: 250 * time.Millisecond}
		connection, err := dialer.DialContext(ctx, "unix", name)
		if err == nil {
			return connection, nil
		}
		last = err
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(75 * time.Millisecond):
		}
	}
	return nil, fmt.Errorf("connect %s: %w", name, last)
}

func (m *Manager) start(ctx context.Context, options Options) (*session, error) {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	if err := m.installServer(); err != nil {
		return nil, err
	}
	id := m.nextID.Add(1) & 0x7fffffff
	processCtx, cancel := context.WithCancel(context.Background())
	command := exec.CommandContext(processCtx, "app_process", serverArgs(options, id)...)
	command.Env = append(os.Environ(), "CLASSPATH="+m.path)
	command.Stdout, command.Stderr = log.Writer(), log.Writer()
	if err := command.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start scrcpy app_process: %w", err)
	}
	s := newSession(command, cancel)
	name := fmt.Sprintf("@scrcpy_%08x", id)
	video, err := dialSocket(ctx, name)
	if err != nil {
		s.close()
		_ = command.Wait()
		return nil, err
	}
	s.video = video
	control, err := dialSocket(ctx, name)
	if err != nil {
		s.close()
		_ = command.Wait()
		return nil, err
	}
	s.control = control
	go s.readDeviceMessages()
	go func() {
		_ = command.Wait()
		s.close()
	}()
	m.mu.Lock()
	previous := m.current
	m.current = s
	m.mu.Unlock()
	if previous != nil {
		previous.close()
	}
	return s, nil
}

func (m *Manager) serveScreen(ctx context.Context, ws *websocket.Conn, definition string) error {
	options, err := optionsFor(definition)
	if err != nil {
		return err
	}
	s, err := m.start(ctx, options)
	if err != nil {
		return err
	}
	defer func() {
		s.close()
		m.mu.Lock()
		if m.current == s {
			m.current = nil
		}
		m.mu.Unlock()
	}()
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		for {
			if _, _, readErr := ws.Read(streamCtx); readErr != nil {
				s.close()
				cancel()
				return
			}
		}
	}()
	buffer := make([]byte, 64*1024)
	for {
		_ = s.video.SetReadDeadline(time.Now().Add(10 * time.Second))
		n, readErr := s.video.Read(buffer)
		if n > 0 {
			writeCtx, writeCancel := context.WithTimeout(streamCtx, 10*time.Second)
			writeErr := ws.Write(writeCtx, websocket.MessageBinary, buffer[:n])
			writeCancel()
			if writeErr != nil {
				return writeErr
			}
		}
		if readErr != nil {
			return readErr
		}
	}
}

func (m *Manager) waitCurrent(ctx context.Context) (*session, error) {
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		m.mu.RLock()
		current := m.current
		m.mu.RUnlock()
		if current != nil && !current.closed() {
			return current, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			return nil, errors.New("screen channel did not create a scrcpy session")
		case <-ticker.C:
		}
	}
}

func (m *Manager) serveControl(ctx context.Context, ws *websocket.Conn) error {
	if !m.acquireControl() {
		return errors.New("scrcpy control channel already active")
	}
	defer m.releaseControl()
	width, height := physicalSize(ctx)
	for {
		var event Event
		if err := readJSON(ctx, ws, &event); err != nil {
			return err
		}
		current, err := m.waitCurrent(ctx)
		if err != nil {
			_ = writeJSON(ctx, ws, map[string]any{"success": false, "error": err.Error()})
			continue
		}
		frame, err := event.frame(width, height)
		if err == nil && event.Type == 0 && event.Milliseconds > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(event.Milliseconds) * time.Millisecond):
			}
		}
		if err == nil && len(frame) > 0 {
			err = current.write(frame)
		}
		if err != nil {
			_ = writeJSON(ctx, ws, map[string]any{"success": false, "error": err.Error()})
			continue
		}
		if event.Type == 4 || (event.Type == 5 && event.Sequence != 0) {
			var response map[string]any
			var responseErr error
			if event.Type == 5 {
				response, responseErr = current.waitClipboardAck(ctx, 2*time.Second, event.Sequence)
			} else {
				response, responseErr = current.waitDeviceEvent(ctx, 2*time.Second, deviceEventTypeMatcher("clipboard"))
			}
			if responseErr != nil {
				_ = writeJSON(ctx, ws, map[string]any{"success": false, "error": responseErr.Error()})
				continue
			}
			if err := writeJSON(ctx, ws, response); err != nil {
				return err
			}
		} else {
			if err := writeJSON(ctx, ws, map[string]any{"success": true, "type": "accepted"}); err != nil {
				return err
			}
		}
	}
}

func (m *Manager) acquireControl() bool {
	m.controlMu.Lock()
	defer m.controlMu.Unlock()
	if m.controlActive {
		return false
	}
	m.controlActive = true
	return true
}

func (m *Manager) releaseControl() {
	m.controlMu.Lock()
	m.controlActive = false
	m.controlMu.Unlock()
}

var sizePattern = regexp.MustCompile(`(?m)(?:Override|Physical) size:\s*(\d+)x(\d+)`)

func physicalSize(ctx context.Context) (uint16, uint16) {
	commandCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, "wm", "size").Output()
	if err != nil {
		return 1080, 1920
	}
	width, height, ok := parsePhysicalSize(string(output))
	if !ok {
		return 1080, 1920
	}
	return width, height
}

func parsePhysicalSize(output string) (uint16, uint16, bool) {
	matches := sizePattern.FindAllStringSubmatch(strings.TrimSpace(output), -1)
	if len(matches) == 0 {
		return 0, 0, false
	}
	match := matches[len(matches)-1]
	width, widthErr := strconv.ParseUint(match[1], 10, 16)
	height, heightErr := strconv.ParseUint(match[2], 10, 16)
	if widthErr != nil || heightErr != nil || width == 0 || height == 0 {
		return 0, 0, false
	}
	return uint16(width), uint16(height), true
}

func readJSON(ctx context.Context, ws *websocket.Conn, value any) error {
	_, data, err := ws.Read(ctx)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}

func writeJSON(ctx context.Context, ws *websocket.Conn, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return ws.Write(writeCtx, websocket.MessageText, data)
}

type session struct {
	video, control net.Conn
	command        *exec.Cmd
	cancel         context.CancelFunc
	done           chan struct{}
	closeOnce      sync.Once
	writeMu        sync.Mutex
	deviceEvents   chan map[string]any
	ackMu          sync.Mutex
	ackPending     map[uint64]struct{}
	ackOrder       []uint64
	ackWaiters     map[uint64][]chan struct{}
	ackSpace       chan struct{}
}

func newSession(command *exec.Cmd, cancel context.CancelFunc) *session {
	return &session{
		command: command, cancel: cancel, done: make(chan struct{}),
		deviceEvents: make(chan map[string]any, 16),
		ackPending:   make(map[uint64]struct{}),
		ackWaiters:   make(map[uint64][]chan struct{}),
		ackSpace:     make(chan struct{}, 1),
	}
}

func (s *session) closed() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

func (s *session) close() {
	s.closeOnce.Do(func() {
		close(s.done)
		if s.video != nil {
			_ = s.video.Close()
		}
		if s.control != nil {
			_ = s.control.Close()
		}
		if s.cancel != nil {
			s.cancel()
		}
		if s.command != nil && s.command.Process != nil {
			_ = s.command.Process.Kill()
		}
	})
}

func (s *session) write(data []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if s.closed() || s.control == nil {
		return net.ErrClosed
	}
	_ = s.control.SetWriteDeadline(time.Now().Add(2 * time.Second))
	for len(data) > 0 {
		written, err := s.control.Write(data)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		data = data[written:]
	}
	return nil
}

func deviceEventTypeMatcher(expected string) func(map[string]any) bool {
	return func(event map[string]any) bool { return event["type"] == expected }
}

func (s *session) waitDeviceEvent(ctx context.Context, timeout time.Duration, matches func(map[string]any) bool) (map[string]any, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case event := <-s.deviceEvents:
			if matches(event) {
				return event, nil
			}
		case <-s.done:
			return nil, errControlSessionClosed
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			return nil, errDeviceResponseTimeout
		}
	}
}

func (s *session) waitClipboardAck(ctx context.Context, timeout time.Duration, sequence uint64) (map[string]any, error) {
	s.ackMu.Lock()
	if _, ok := s.ackPending[sequence]; ok {
		delete(s.ackPending, sequence)
		for index, pending := range s.ackOrder {
			if pending == sequence {
				s.ackOrder = append(s.ackOrder[:index], s.ackOrder[index+1:]...)
				break
			}
		}
		s.ackMu.Unlock()
		s.signalAckSpace()
		return clipboardAckEvent(sequence), nil
	}
	waiter := make(chan struct{}, 1)
	s.ackWaiters[sequence] = append(s.ackWaiters[sequence], waiter)
	s.ackMu.Unlock()
	s.signalAckSpace()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-waiter:
		return clipboardAckEvent(sequence), nil
	case <-s.done:
		s.removeAckWaiter(sequence, waiter)
		return nil, errControlSessionClosed
	case <-ctx.Done():
		if !s.removeAckWaiter(sequence, waiter) {
			<-waiter
			return clipboardAckEvent(sequence), nil
		}
		return nil, ctx.Err()
	case <-timer.C:
		if !s.removeAckWaiter(sequence, waiter) {
			<-waiter
			return clipboardAckEvent(sequence), nil
		}
		return nil, errDeviceResponseTimeout
	}
}

func clipboardAckEvent(sequence uint64) map[string]any {
	return map[string]any{"type": "clipboardAck", "sequence": sequence}
}

func (s *session) removeAckWaiter(sequence uint64, target chan struct{}) bool {
	s.ackMu.Lock()
	defer s.ackMu.Unlock()
	waiters := s.ackWaiters[sequence]
	for index, waiter := range waiters {
		if waiter != target {
			continue
		}
		waiters = append(waiters[:index], waiters[index+1:]...)
		if len(waiters) == 0 {
			delete(s.ackWaiters, sequence)
		} else {
			s.ackWaiters[sequence] = waiters
		}
		return true
	}
	return false
}

func (s *session) readDeviceMessages() {
	for {
		var kind [1]byte
		if _, err := io.ReadFull(s.control, kind[:]); err != nil {
			return
		}
		switch kind[0] {
		case 0:
			var lengthBytes [4]byte
			if _, err := io.ReadFull(s.control, lengthBytes[:]); err != nil {
				return
			}
			length := binary.BigEndian.Uint32(lengthBytes[:])
			if length > maxMessage {
				return
			}
			data := make([]byte, length)
			if _, err := io.ReadFull(s.control, data); err != nil {
				return
			}
			s.publish(map[string]any{"type": "clipboard", "text": string(data)})
		case 1:
			var sequence [8]byte
			if _, err := io.ReadFull(s.control, sequence[:]); err != nil {
				return
			}
			if !s.publishClipboardAck(binary.BigEndian.Uint64(sequence[:])) {
				return
			}
		case 2:
			var header [4]byte
			if _, err := io.ReadFull(s.control, header[:]); err != nil {
				return
			}
			if _, err := io.CopyN(io.Discard, s.control, int64(binary.BigEndian.Uint16(header[2:]))); err != nil {
				return
			}
		default:
			return
		}
	}
}

func (s *session) publishClipboardAck(sequence uint64) bool {
	for {
		if s.closed() {
			return false
		}
		s.ackMu.Lock()
		waiters := s.ackWaiters[sequence]
		if len(waiters) > 0 {
			waiter := waiters[0]
			if len(waiters) == 1 {
				delete(s.ackWaiters, sequence)
			} else {
				s.ackWaiters[sequence] = waiters[1:]
			}
			waiter <- struct{}{}
			s.ackMu.Unlock()
			return true
		}
		if _, exists := s.ackPending[sequence]; exists {
			s.ackMu.Unlock()
			return true
		}
		if len(s.ackOrder) < clipboardAckQueueSize {
			s.ackPending[sequence] = struct{}{}
			s.ackOrder = append(s.ackOrder, sequence)
			s.ackMu.Unlock()
			return true
		}
		s.ackMu.Unlock()
		select {
		case <-s.done:
			return false
		case <-s.ackSpace:
		}
	}
}

func (s *session) signalAckSpace() {
	select {
	case s.ackSpace <- struct{}{}:
	default:
	}
}

func (s *session) publish(event map[string]any) {
	select {
	case s.deviceEvents <- event:
	default:
		select {
		case <-s.deviceEvents:
		default:
		}
		select {
		case s.deviceEvents <- event:
		default:
		}
	}
}
