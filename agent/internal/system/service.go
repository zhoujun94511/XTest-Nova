package system

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/platform"
)

type ProcessInfo struct {
	PID         int      `json:"pid"`
	PPID        int      `json:"ppid"`
	ThreadCount int      `json:"threadCount"`
	Cmdline     []string `json:"cmdline"`
	Name        string   `json:"name"`
}
type CPUInfo struct {
	PID           int     `json:"pid"`
	PIDs          []int   `json:"pids,omitempty"`
	User          uint64  `json:"user"`
	System        uint64  `json:"system"`
	Percent       float64 `json:"percent"`
	CorePercent   float64 `json:"corePercent"`
	SystemPercent float64 `json:"systemPercent"`
	CoreCount     int     `json:"coreCount"`
	Scope         string  `json:"scope,omitempty"`
	State         string  `json:"state,omitempty"`
}
type NetInfo struct {
	Tx               int64     `json:"tx"`
	Rx               int64     `json:"rx"`
	Total            int64     `json:"total"`
	TxBytesPerSecond *float64  `json:"txBytesPerSecond,omitempty"`
	RxBytesPerSecond *float64  `json:"rxBytesPerSecond,omitempty"`
	RateWindowMillis int64     `json:"rateWindowMillis,omitempty"`
	RateSampledAt    time.Time `json:"rateSampledAt,omitempty"`
	UID              int       `json:"uid,omitempty"`
	Scope            string    `json:"scope,omitempty"`
	Source           string    `json:"source,omitempty"`
}
type NetStatus struct {
	DownloadSpeed   string `json:"downloadSpeed"`
	UploadSpeed     string `json:"uploadSpeed"`
	Connected       bool   `json:"connected"`
	MobileConnected bool   `json:"mobileConnected"`
	ConnectedType   int    `json:"connectedType"`
}
type Storage struct {
	Total     int64 `json:"total"`
	Available int64 `json:"available"`
}
type Memory struct {
	Total     int64 `json:"total"`
	Free      int64 `json:"free"`
	Available int64 `json:"available"`
}
type Performance struct {
	CPU                      CPUInfo                `json:"cpuinfo"`
	Memory                   map[string]int         `json:"memoinfo"`
	FPS                      *float64               `json:"fps"`
	RenderFPS                *float64               `json:"renderFps,omitempty"`
	Frame                    *FrameInfo             `json:"frame,omitempty"`
	GPU                      *GPUInfo               `json:"gpu"`
	Battery                  BatteryInfo            `json:"battery"`
	Network                  NetInfo                `json:"network"`
	Sources                  map[string]string      `json:"sources,omitempty"`
	Errors                   map[string]string      `json:"errors,omitempty"`
	MetricStates             map[string]MetricState `json:"metricStates,omitempty"`
	CollectedAt              time.Time              `json:"collectedAt"`
	CollectionDurationMillis int64                  `json:"collectionDurationMillis"`
}
type MetricState struct {
	State  string `json:"state"`
	Source string `json:"source,omitempty"`
	Reason string `json:"reason,omitempty"`
}
type GPUInfo struct {
	Percent float64 `json:"percent"`
	Source  string  `json:"source"`
}
type FrameInfo struct {
	RenderedFrames int64   `json:"renderedFrames"`
	JankCount      int64   `json:"jankCount"`
	JankRate       float64 `json:"jankRate"`
	Source         string  `json:"source"`
	State          string  `json:"state,omitempty"`
}
type BatteryInfo struct {
	CurrentMA   *float64 `json:"currentMa"`
	Status      string   `json:"status,omitempty"`
	Level       int      `json:"levelPercent"`
	Temperature float64  `json:"temperatureC"`
	Source      string   `json:"source,omitempty"`
}
type IMEStatus struct {
	Current string   `json:"currentIme"`
	Enabled []string `json:"enabledImes"`
}
type WebView struct {
	PID        int    `json:"pid"`
	Name       string `json:"name"`
	SocketPath string `json:"socketPath"`
}
type Service struct {
	executor          platform.Executor
	timeout           time.Duration
	surfaceMu         sync.Mutex
	surfaceStatsReady bool
	defaultFrames     *frameRateState
	now               func() time.Time
}

func New(executor platform.Executor, timeout time.Duration) *Service {
	service := &Service{
		executor: executor,
		timeout:  timeout,
		now:      time.Now,
	}
	service.defaultFrames = newFrameRateState()
	return service
}
func (s *Service) run(ctx context.Context, name string, args ...string) (string, error) {
	c, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.executor.Run(c, name, args...)
}

// Run exposes the same bounded command transport to higher-level, typed
// diagnostics such as startup-time measurement.
func (s *Service) Run(ctx context.Context, name string, args ...string) (string, error) {
	return s.run(ctx, name, args...)
}
func (s *Service) probe(ctx context.Context, name string, args ...string) (string, error) {
	timeout := 2 * time.Second
	if s.timeout > 0 && s.timeout < timeout {
		timeout = s.timeout
	}
	c, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return s.executor.Run(c, name, args...)
}
func (s *Service) Processes() ([]ProcessInfo, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	result := make([]ProcessInfo, 0)
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || !entry.IsDir() {
			continue
		}
		cmdData, _ := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		cmdline := splitNull(cmdData)
		statusData, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
		if err != nil {
			continue
		}
		status := parseStatus(string(statusData))
		name := status["Name"]
		if len(cmdline) == 1 {
			name = cmdline[0]
		}
		ppid, _ := strconv.Atoi(status["PPid"])
		threads, _ := strconv.Atoi(status["Threads"])
		result = append(result, ProcessInfo{PID: pid, PPID: ppid, ThreadCount: threads, Cmdline: cmdline, Name: name})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].PID < result[j].PID })
	return result, nil
}
func splitNull(data []byte) []string {
	parts := strings.Split(strings.TrimRight(string(data), "\x00"), "\x00")
	if len(parts) == 1 && parts[0] == "" {
		return []string{}
	}
	return parts
}
func parseStatus(data string) map[string]string {
	result := map[string]string{}
	for _, line := range strings.Split(data, "\n") {
		if key, value, ok := strings.Cut(line, ":"); ok {
			result[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	return result
}
func (s *Service) PID(ctx context.Context, name string) (int, error) {
	if name == "" || strings.ContainsAny(name, "/\\\x00 \t\r\n") {
		return 0, fmt.Errorf("invalid process name")
	}
	out, err := s.run(ctx, "pidof", name)
	if err != nil {
		return 0, fmt.Errorf("package not found")
	}
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return 0, fmt.Errorf("package not found")
	}
	return strconv.Atoi(fields[0])
}

// PIDs returns the main process and any colon-suffixed package processes.
func (s *Service) PIDs(ctx context.Context, name string) ([]int, error) {
	mainPID, err := s.PID(ctx, name)
	if err != nil {
		return nil, err
	}
	pids := []int{mainPID}
	// pidof matches the exact main process name. Enrich it with Android's
	// conventional package:service processes where ps supports selected columns.
	if processes, psErr := s.run(ctx, "ps", "-A", "-o", "PID,NAME"); psErr == nil {
		pids = append(pids, ParsePackagePIDs(processes, name)...)
	}
	extras := append([]int(nil), pids[1:]...)
	sort.Ints(extras)
	unique := []int{mainPID}
	for _, pid := range extras {
		if pid != mainPID && (len(unique) == 1 || unique[len(unique)-1] != pid) {
			unique = append(unique, pid)
		}
	}
	return unique, nil
}

func ParsePackagePIDs(output, packageName string) []int {
	result := make([]int, 0)
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		processName := fields[len(fields)-1]
		if processName != packageName && !strings.HasPrefix(processName, packageName+":") {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err == nil && pid > 0 {
			result = append(result, pid)
		}
	}
	return result
}

var memoryPattern = regexp.MustCompile(`(?m)^\s*(\w[\w ]+):\s*(\d+)`)

func ParseAppMemory(output string) (map[string]int, error) {
	index := strings.Index(output, "App Summary")
	if index < 0 {
		return nil, fmt.Errorf("dumpsys meminfo has no App Summary")
	}
	result := map[string]int{}
	for _, match := range memoryPattern.FindAllStringSubmatch(output[index:], -1) {
		value, _ := strconv.Atoi(match[2])
		result[strings.ToLower(strings.TrimSpace(match[1]))] = value
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("invalid dumpsys meminfo output")
	}
	return result, nil
}
func (s *Service) AppMemory(ctx context.Context, name string) (map[string]int, error) {
	if name == "" || strings.ContainsAny(name, "/\\\x00\r\n") {
		return nil, fmt.Errorf("invalid package")
	}
	out, err := s.run(ctx, "dumpsys", "meminfo", "--local", name)
	if err != nil {
		return nil, err
	}
	return ParseAppMemory(out)
}

type cpuStat struct{ total, idle, user, system uint64 }

type packageCPUSnapshot struct {
	pids         []int
	total, idle  uint64
	user, system uint64
}

func readCPUStat(pid int) (cpuStat, error) {
	process, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return cpuStat{}, err
	}
	end := strings.LastIndex(string(process), ")")
	if end < 0 {
		return cpuStat{}, fmt.Errorf("invalid process stat")
	}
	fields := strings.Fields(string(process)[end+1:])
	if len(fields) < 13 {
		return cpuStat{}, fmt.Errorf("invalid process stat")
	}
	user, _ := strconv.ParseUint(fields[11], 10, 64)
	system, _ := strconv.ParseUint(fields[12], 10, 64)
	global, err := os.ReadFile("/proc/stat")
	if err != nil {
		return cpuStat{}, err
	}
	line := strings.SplitN(string(global), "\n", 2)[0]
	values := strings.Fields(line)
	if len(values) < 5 || values[0] != "cpu" {
		return cpuStat{}, fmt.Errorf("invalid system stat")
	}
	var total, idle uint64
	for index, value := range values[1:] {
		// guest and guest_nice are already included in user/nice on Linux.
		// Summing them again inflates the denominator. Treat iowait as idle so
		// the reported system load is CPU work rather than scheduler occupancy.
		if index >= 8 {
			break
		}
		parsed, _ := strconv.ParseUint(value, 10, 64)
		total += parsed
		if index == 3 || index == 4 {
			idle += parsed
		}
	}
	return cpuStat{total: total, idle: idle, user: user, system: system}, nil
}
func (s *Service) packageCPUSnapshot(ctx context.Context, name string) (packageCPUSnapshot, error) {
	pids, err := s.PIDs(ctx, name)
	if err != nil {
		return packageCPUSnapshot{}, err
	}
	result := packageCPUSnapshot{pids: append([]int(nil), pids...)}
	for index, pid := range pids {
		stat, readErr := readCPUStat(pid)
		if readErr != nil {
			return packageCPUSnapshot{}, readErr
		}
		result.user += stat.user
		result.system += stat.system
		if index == 0 {
			result.total, result.idle = stat.total, stat.idle
		}
	}
	return result, nil
}

func samePIDs(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func cpuInfoBetween(before, after packageCPUSnapshot) CPUInfo {
	info := CPUInfo{PIDs: append([]int(nil), after.pids...), User: after.user, System: after.system, CoreCount: runtime.NumCPU(), Scope: "package_processes", State: "warming_up"}
	if len(after.pids) > 0 {
		info.PID = after.pids[0]
	}
	if !samePIDs(before.pids, after.pids) || after.total <= before.total || after.idle < before.idle || after.user < before.user || after.system < before.system {
		return info
	}
	deltaTotal := after.total - before.total
	deltaProcess := (after.user + after.system) - (before.user + before.system)
	info.Percent = 100 * float64(deltaProcess) / float64(deltaTotal)
	info.CorePercent = info.Percent * float64(info.CoreCount)
	info.SystemPercent = 100 * float64(deltaTotal-(after.idle-before.idle)) / float64(deltaTotal)
	info.State = "measured"
	if deltaProcess == 0 {
		info.State = "idle"
	}
	return info
}

func (s *Service) CPU(ctx context.Context, name string) (CPUInfo, error) {
	before, err := s.packageCPUSnapshot(ctx, name)
	if err != nil {
		return CPUInfo{}, err
	}
	select {
	case <-ctx.Done():
		return CPUInfo{}, ctx.Err()
	case <-time.After(100 * time.Millisecond):
	}
	after, err := s.packageCPUSnapshot(ctx, name)
	if err != nil {
		return CPUInfo{}, err
	}
	return cpuInfoBetween(before, after), nil
}

// NetworkForPID is retained for the legacy process endpoint. Linux exposes
// namespace-wide interface counters here, not traffic attributable to the PID;
// performance sessions therefore use NetworkForPackage instead.
func (s *Service) NetworkForPID(pid int) (NetInfo, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/net/dev", pid))
	if err != nil {
		return NetInfo{}, err
	}
	var info NetInfo
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "lo" {
			continue
		}
		fields := strings.Fields(parts[1])
		if len(fields) < 16 {
			continue
		}
		rx, _ := strconv.ParseInt(fields[0], 10, 64)
		tx, _ := strconv.ParseInt(fields[8], 10, 64)
		info.Rx += rx
		info.Tx += tx
	}
	info.Total = info.Rx + info.Tx
	return info, nil
}

type frameSample struct {
	frames int64
	janky  int64
	at     time.Time
}

var renderedFramesPattern = regexp.MustCompile(`(?m)^\s*Total frames rendered:\s*(\d+)\s*$`)
var jankyFramesPattern = regexp.MustCompile(`(?m)^\s*Janky frames(?: \([^)]*\))?:\s*(\d+)`)
var surfaceTotalFramesPattern = regexp.MustCompile(`(?m)^\s*totalFrames\s*=\s*(\d+)\s*$`)
var surfaceLayerIDPattern = regexp.MustCompile(`#(\d+)\s*$`)

func ParseRenderedFrames(output string) (int64, error) {
	match := renderedFramesPattern.FindStringSubmatch(output)
	if len(match) != 2 {
		return 0, fmt.Errorf("dumpsys gfxinfo has no rendered frame counter")
	}
	return strconv.ParseInt(match[1], 10, 64)
}

func ParseJankyFrames(output string) (int64, error) {
	match := jankyFramesPattern.FindStringSubmatch(output)
	if len(match) != 2 {
		return 0, fmt.Errorf("dumpsys gfxinfo has no janky frame counter")
	}
	return strconv.ParseInt(match[1], 10, 64)
}

// ParseSurfaceFrames returns the cumulative frame counter for the newest
// SurfaceFlinger layer whose name contains the target package. Timestats retains
// destroyed layers, so choosing the largest counter can pin FPS to an old layer
// after an activity restart. Android layer IDs are monotonic; dumps without IDs
// fall back to the last matching block.
func surfaceLayerMatchScore(line, packageName string) int {
	if !strings.Contains(line, packageName) {
		return -1
	}
	score := 1
	lower := strings.ToLower(line)
	if strings.Contains(lower, "surfaceview") {
		score += 4
	}
	if strings.Contains(lower, "blastbufferqueue") {
		score += 3
	}
	if strings.Contains(line, packageName+"/") {
		score += 2
	}
	return score
}

func ParseSurfaceFrames(output, packageName string) (int64, error) {
	if packageName == "" {
		return 0, fmt.Errorf("empty package name")
	}
	var selectedFrames int64 = -1
	var selectedLayerID int64 = -1
	var selectedScore = -1
	for _, block := range strings.Split(output, "layerName = ")[1:] {
		line, remainder, _ := strings.Cut(block, "\n")
		score := surfaceLayerMatchScore(strings.TrimSpace(line), packageName)
		if score < 0 {
			continue
		}
		match := surfaceTotalFramesPattern.FindStringSubmatch(remainder)
		if len(match) != 2 {
			continue
		}
		frames, err := strconv.ParseInt(match[1], 10, 64)
		if err != nil {
			continue
		}
		layerID := int64(-1)
		if idMatch := surfaceLayerIDPattern.FindStringSubmatch(strings.TrimSpace(line)); len(idMatch) == 2 {
			if parsed, parseErr := strconv.ParseInt(idMatch[1], 10, 64); parseErr == nil {
				layerID = parsed
			}
		}
		if score > selectedScore || (score == selectedScore && layerID >= selectedLayerID) {
			selectedScore = score
			selectedLayerID = layerID
			selectedFrames = frames
		}
	}
	if selectedFrames < 0 {
		return 0, fmt.Errorf("SurfaceFlinger timestats has no layer for package")
	}
	return selectedFrames, nil
}

func frameRateDelta(previous frameSample, exists bool, frames int64, now time.Time) *float64 {
	if !exists || frames < previous.frames {
		return nil
	}
	seconds := now.Sub(previous.at).Seconds()
	if seconds <= 0 {
		return nil
	}
	value := float64(frames-previous.frames) / seconds
	if value < 0 || value > 1000 {
		return nil
	}
	return &value
}

func (s *Service) surfaceFrameRate(ctx context.Context, name string, state *frameRateState) (*float64, error) {
	s.surfaceMu.Lock()
	ready := s.surfaceStatsReady
	if !ready {
		s.surfaceStatsReady = true
	}
	s.surfaceMu.Unlock()
	if !ready {
		if _, err := s.probe(ctx, "dumpsys", "SurfaceFlinger", "--timestats", "-clear"); err != nil {
			s.surfaceMu.Lock()
			s.surfaceStatsReady = false
			s.surfaceMu.Unlock()
			return nil, err
		}
		if _, err := s.probe(ctx, "dumpsys", "SurfaceFlinger", "--timestats", "-enable"); err != nil {
			s.surfaceMu.Lock()
			s.surfaceStatsReady = false
			s.surfaceMu.Unlock()
			return nil, err
		}
		state.mu.Lock()
		state.surface = map[string]frameSample{}
		state.mu.Unlock()
		return nil, nil
	}
	out, err := s.probe(ctx, "dumpsys", "SurfaceFlinger", "--timestats", "-dump")
	if err != nil {
		return nil, err
	}
	frames, err := ParseSurfaceFrames(out, name)
	if err != nil {
		return nil, err
	}
	// Timestamp the counter after the dump has completed. Measuring from probe
	// start shortens the denominator by the command latency and can inflate FPS.
	now := s.now()
	state.mu.Lock()
	previous, exists := state.surface[name]
	state.surface[name] = frameSample{frames: frames, at: now}
	state.mu.Unlock()
	return frameRateDelta(previous, exists, frames, now), nil
}

// FrameRate returns screen presentation FPS when SurfaceFlinger timestats is
// available. gfxinfo remains a separate render-throughput fallback. The first
// sample and counter resets intentionally return nil instead of a fabricated 0.
func (s *Service) FrameRate(ctx context.Context, name string) (*float64, error) {
	value, _, _, err := s.frameRate(ctx, name, s.defaultFrames)
	return value, err
}

func (s *Service) frameRate(ctx context.Context, name string, state *frameRateState) (*float64, *FrameInfo, string, error) {
	// Sampling is stateful and SurfaceFlinger timestats is process-global.
	// Serialize callers so an older probe cannot overwrite a newer baseline.
	state.operation.Lock()
	defer state.operation.Unlock()
	var renderValue *float64
	var frame *FrameInfo
	var gfxErr error
	out, probeErr := s.probe(ctx, "dumpsys", "gfxinfo", name)
	if probeErr != nil {
		gfxErr = probeErr
	} else if frames, parseErr := ParseRenderedFrames(out); parseErr != nil {
		gfxErr = parseErr
	} else {
		// Timestamp the counter after the dump has completed so command latency is
		// not accidentally converted into a higher frame rate.
		now := s.now()
		janky, jankErr := ParseJankyFrames(out)
		if jankErr != nil {
			janky = -1
		}
		state.mu.Lock()
		previous, exists := state.view[name]
		if !exists && len(state.view) >= 128 {
			state.view = map[string]frameSample{}
		}
		state.view[name] = frameSample{frames: frames, janky: janky, at: now}
		state.mu.Unlock()
		renderValue = frameRateDelta(previous, exists, frames, now)
		if exists && janky >= 0 && previous.janky >= 0 && frames >= previous.frames && janky >= previous.janky {
			deltaFrames, deltaJank := frames-previous.frames, janky-previous.janky
			rate := float64(0)
			if deltaFrames > 0 {
				rate = 100 * float64(deltaJank) / float64(deltaFrames)
			}
			frameState := "measured"
			if deltaFrames == 0 {
				frameState = "idle"
			}
			frame = &FrameInfo{RenderedFrames: deltaFrames, JankCount: deltaJank, JankRate: rate, Source: "gfxinfo", State: frameState}
		}
	}

	// SurfaceFlinger counts frames presented by the selected target layer and
	// is therefore the authoritative screen-FPS source. gfxinfo's package-wide
	// rendered counter can aggregate multiple ViewRoots (for example a game and
	// an embedded ad), so its rate is diagnostic render throughput, not display
	// FPS, and must never override an available presentation counter.
	if surfaceValue, surfaceErr := s.surfaceFrameRate(ctx, name, state); surfaceErr == nil {
		return surfaceValue, frame, "surfaceflinger-timestats", nil
	} else if gfxErr != nil {
		return nil, frame, "surfaceflinger-timestats", fmt.Errorf("presented FPS unavailable: %v; gfxinfo render rate unavailable: %w", surfaceErr, gfxErr)
	}
	return renderValue, frame, "gfxinfo-render-rate", nil
}

var percentValuePattern = regexp.MustCompile(`(-?\d+(?:\.\d+)?)`)

func parsePercentValue(value string) (float64, error) {
	match := percentValuePattern.FindStringSubmatch(value)
	if len(match) != 2 {
		return 0, fmt.Errorf("percentage value not found")
	}
	parsed, err := strconv.ParseFloat(match[1], 64)
	if err != nil || parsed < 0 || parsed > 100 {
		return 0, fmt.Errorf("invalid percentage value")
	}
	return parsed, nil
}

func (s *Service) GPU() *GPUInfo {
	for _, candidate := range []struct{ path, source string }{
		{"/sys/class/kgsl/kgsl-3d0/gpu_busy_percentage", "kgsl"},
		{"/sys/kernel/gpu/gpu_busy", "kernel-gpu"},
	} {
		data, err := os.ReadFile(candidate.path)
		if err != nil {
			continue
		}
		value, err := parsePercentValue(string(data))
		if err == nil {
			return &GPUInfo{Percent: value, Source: candidate.source}
		}
	}
	return nil
}

var batteryValuePattern = regexp.MustCompile(`(?m)^\s*([\w ]+):\s*(-?\d+)\s*$`)

func ParseBattery(output string) BatteryInfo {
	values := map[string]int64{}
	for _, match := range batteryValuePattern.FindAllStringSubmatch(output, -1) {
		value, _ := strconv.ParseInt(match[2], 10, 64)
		values[strings.ToLower(strings.TrimSpace(match[1]))] = value
	}
	statuses := map[int64]string{1: "unknown", 2: "charging", 3: "discharging", 4: "not_charging", 5: "full"}
	return BatteryInfo{Status: statuses[values["status"]], Level: int(values["level"]), Temperature: float64(values["temperature"]) / 10}
}

func normalizeCurrentMA(value int64) float64 {
	if value > 10000 || value < -10000 {
		return float64(value) / 1000
	}
	return float64(value)
}

func (s *Service) Battery(ctx context.Context) BatteryInfo {
	result := BatteryInfo{}
	if out, err := s.probe(ctx, "dumpsys", "battery"); err == nil {
		result = ParseBattery(out)
	}
	for _, path := range []string{
		"/sys/class/power_supply/battery/current_now",
		"/sys/class/power_supply/battery/current_avg",
		"/sys/class/power_supply/bms/current_now",
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		value, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
		if err == nil {
			current := normalizeCurrentMA(value)
			result.CurrentMA, result.Source = &current, path
			break
		}
	}
	return result
}

func (s *Service) Performance(ctx context.Context, name string) (Performance, error) {
	return s.collectPerformance(ctx, name, s.CPU, func(frameContext context.Context, packageName string) (*float64, *FrameInfo, string, error) {
		return s.frameRate(frameContext, packageName, s.defaultFrames)
	}, s.NetworkForPackage)
}

func (s *Service) collectPerformance(
	ctx context.Context,
	name string,
	cpuProbe func(context.Context, string) (CPUInfo, error),
	frameProbe func(context.Context, string) (*float64, *FrameInfo, string, error),
	networkProbe func(context.Context, string) (NetInfo, error),
) (Performance, error) {
	started := time.Now()
	pid, err := s.PID(ctx, name)
	if err != nil {
		return Performance{}, err
	}
	result := Performance{Memory: map[string]int{}, Sources: map[string]string{}, Errors: map[string]string{}, MetricStates: map[string]MetricState{}}
	result.CPU.PID = pid
	var fps *float64
	var frame *FrameInfo
	var fpsSource string
	var gpu *GPUInfo
	var battery BatteryInfo
	var cpu CPUInfo
	var memory map[string]int
	var network NetInfo
	var cpuErr, memoryErr, networkErr, fpsErr error
	var probes sync.WaitGroup
	probes.Add(6)
	go func() { defer probes.Done(); cpu, cpuErr = cpuProbe(ctx, name) }()
	go func() { defer probes.Done(); memory, memoryErr = s.AppMemory(ctx, name) }()
	go func() { defer probes.Done(); network, networkErr = networkProbe(ctx, name) }()
	go func() { defer probes.Done(); fps, frame, fpsSource, fpsErr = frameProbe(ctx, name) }()
	go func() { defer probes.Done(); gpu = s.GPU() }()
	go func() { defer probes.Done(); battery = s.Battery(ctx) }()
	probes.Wait()
	result.CPU, result.Memory, result.Frame, result.GPU, result.Battery, result.Network = cpu, memory, frame, gpu, battery, network
	if fpsSource == "gfxinfo-render-rate" {
		result.RenderFPS = fps
	} else {
		result.FPS = fps
	}
	result.Sources["cpu"], result.Sources["memory"], result.Sources["battery"] = "proc-stat", "dumpsys-meminfo", "dumpsys-battery+sysfs"
	cpuState := cpu.State
	if cpuState == "" {
		cpuState = "measured"
	}
	result.MetricStates["cpu"] = MetricState{State: cpuState, Source: result.Sources["cpu"]}
	result.MetricStates["memory"] = MetricState{State: "measured", Source: result.Sources["memory"]}
	if fpsSource == "gfxinfo-render-rate" {
		result.Sources["renderRate"] = "gfxinfo"
	} else if fpsSource != "" {
		result.Sources["fps"] = fpsSource
	}
	if fpsSource == "gfxinfo-render-rate" {
		renderState := "warming_up"
		if fps != nil {
			renderState = "measured"
			if *fps == 0 {
				renderState = "idle"
			}
		}
		result.MetricStates["render_rate"] = MetricState{State: renderState, Source: "gfxinfo"}
		result.MetricStates["fps"] = MetricState{State: "unsupported", Reason: "presented-frame source unavailable; gfxinfo render rate is reported separately"}
	} else {
		fpsState := "warming_up"
		if fps != nil {
			fpsState = "measured"
			if *fps == 0 {
				fpsState = "idle"
			}
		}
		result.MetricStates["fps"] = MetricState{State: fpsState, Source: fpsSource}
	}
	if frame != nil {
		frameState := frame.State
		if frameState == "" {
			frameState = "measured"
		}
		result.MetricStates["jank"] = MetricState{State: frameState, Source: frame.Source}
	} else {
		result.MetricStates["jank"] = MetricState{State: "warming_up", Source: fpsSource}
	}
	if gpu != nil {
		result.Sources["gpu"] = gpu.Source
		state := "measured"
		if gpu.Percent == 0 {
			state = "idle"
		}
		result.MetricStates["gpu"] = MetricState{State: state, Source: gpu.Source}
	} else {
		result.MetricStates["gpu"] = MetricState{State: "unsupported", Reason: "supported GPU utilization source unavailable"}
	}
	if battery.CurrentMA != nil {
		result.MetricStates["battery_current"] = MetricState{State: "measured", Source: battery.Source}
	} else {
		result.MetricStates["battery_current"] = MetricState{State: "unsupported", Reason: "battery current source unavailable"}
	}
	if network.Source != "" {
		result.Sources["network"] = network.Source
	}
	result.MetricStates["network"] = MetricState{State: "measured", Source: network.Source}
	networkRateState := MetricState{State: "warming_up", Source: network.Source, Reason: "waiting for the next UID network counter sample"}
	if network.RxBytesPerSecond != nil && network.TxBytesPerSecond != nil {
		networkRateState = MetricState{State: "measured", Source: network.Source}
		if *network.RxBytesPerSecond == 0 && *network.TxBytesPerSecond == 0 {
			networkRateState.State = "idle"
		}
	}
	result.MetricStates["network_rate"] = networkRateState
	for key, probeErr := range map[string]error{"cpu": cpuErr, "memory": memoryErr, "network": networkErr, "fps": fpsErr} {
		if probeErr != nil {
			result.Errors[key] = probeErr.Error()
			result.MetricStates[key] = MetricState{State: "failed", Source: result.Sources[key], Reason: probeErr.Error()}
			if key == "network" {
				result.MetricStates["network_rate"] = MetricState{State: "failed", Source: result.Sources[key], Reason: probeErr.Error()}
			}
		}
	}
	result.CollectedAt = time.Now().UTC()
	result.CollectionDurationMillis = time.Since(started).Milliseconds()
	return result, nil
}
func (s *Service) DeviceMemory() (Memory, error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return Memory{}, err
	}
	values := map[string]int64{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			value, _ := strconv.ParseInt(fields[1], 10, 64)
			values[strings.TrimSuffix(fields[0], ":")] = value
		}
	}
	return Memory{Total: values["MemTotal"], Free: values["MemFree"], Available: values["MemAvailable"]}, nil
}
func (s *Service) Storage(ctx context.Context) (Storage, error) {
	out, err := s.run(ctx, "df", "-k", "/data")
	if err != nil {
		return Storage{}, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		return Storage{}, fmt.Errorf("invalid df output")
	}
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 4 {
		return Storage{}, fmt.Errorf("invalid df output")
	}
	total, e1 := strconv.ParseInt(fields[1], 10, 64)
	available, e2 := strconv.ParseInt(fields[3], 10, 64)
	if e1 != nil || e2 != nil {
		return Storage{}, fmt.Errorf("invalid df values")
	}
	return Storage{Total: total * 1024, Available: available * 1024}, nil
}
func (s *Service) NetworkStatus(ctx context.Context) (NetStatus, error) {
	out, err := s.run(ctx, "dumpsys", "connectivity")
	if err != nil {
		return NetStatus{}, err
	}
	upper := strings.ToUpper(out)
	status := NetStatus{ConnectedType: -1, Connected: strings.Contains(upper, "CONNECTED") || strings.Contains(upper, "ACTIVE NETWORK")}
	if status.Connected && strings.Contains(upper, "WIFI") {
		status.ConnectedType = 1
	} else if status.Connected && strings.Contains(upper, "MOBILE") {
		status.ConnectedType = 0
		status.MobileConnected = true
	}
	return status, nil
}
func (s *Service) IME(ctx context.Context) (IMEStatus, error) {
	current, err := s.run(ctx, "settings", "get", "secure", "default_input_method")
	if err != nil {
		return IMEStatus{}, err
	}
	enabled, err := s.run(ctx, "ime", "list", "-s")
	if err != nil {
		return IMEStatus{}, err
	}
	values := make([]string, 0)
	for _, line := range strings.Split(enabled, "\n") {
		if value := strings.TrimSpace(line); value != "" {
			values = append(values, value)
		}
	}
	return IMEStatus{Current: strings.TrimSpace(current), Enabled: values}, nil
}

var imePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.]+/[A-Za-z0-9_.$]+$`)

func (s *Service) SetIME(ctx context.Context, value string) (string, error) {
	if !imePattern.MatchString(value) {
		return "", fmt.Errorf("invalid IME component")
	}
	return s.run(ctx, "ime", "set", value)
}
func (s *Service) WebViews(packageName string) ([]WebView, error) {
	data, err := os.ReadFile("/proc/net/unix")
	if err != nil {
		return nil, err
	}
	sockets := make([]string, 0)
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 7 {
			value := strings.TrimPrefix(fields[len(fields)-1], "@")
			if strings.Contains(value, "devtools_remote") {
				sockets = append(sockets, value)
			}
		}
	}
	processes, err := s.Processes()
	if err != nil {
		return nil, err
	}
	result := make([]WebView, 0)
	for _, process := range processes {
		if packageName != "" && process.Name != packageName && !strings.HasPrefix(process.Name, packageName+":") {
			continue
		}
		suffix := "_" + strconv.Itoa(process.PID)
		for _, socket := range sockets {
			if strings.HasSuffix(socket, suffix) || (packageName == "com.android.browser" && socket == "chrome_devtools_remote") {
				result = append(result, WebView{PID: process.PID, Name: process.Name, SocketPath: socket})
			}
		}
	}
	return result, nil
}

var ipv4Pattern = regexp.MustCompile(`\binet\s+(\d+\.\d+\.\d+\.\d+)/`)
var rotationPattern = regexp.MustCompile(`(?m)(?:SurfaceOrientation|orientation)\s*[:=]\s*([0-3])\b`)

func (s *Service) WLANIP(ctx context.Context) (string, error) {
	out, err := s.run(ctx, "ip", "-4", "addr", "show", "wlan0")
	if err != nil {
		return "", err
	}
	match := ipv4Pattern.FindStringSubmatch(out)
	if len(match) != 2 {
		return "", fmt.Errorf("wlan0 IPv4 address not found")
	}
	return match[1], nil
}
func (s *Service) Rotation(ctx context.Context) (int, error) {
	out, err := s.run(ctx, "dumpsys", "input")
	if err != nil {
		return 0, err
	}
	match := rotationPattern.FindStringSubmatch(out)
	if len(match) != 2 {
		return 0, fmt.Errorf("display rotation not found")
	}
	return strconv.Atoi(match[1])
}
