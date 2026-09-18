package device

import (
	"context"
	"fmt"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/buildinfo"
	"github.com/zhoujun94511/xtest-nova/agent/internal/platform"
)

type Display struct {
	Width    int `json:"width"`
	Height   int `json:"height"`
	Rotation int `json:"rotation"`
}
type Density struct {
	Density int `json:"density"`
}
type Battery struct {
	Level       int  `json:"level"`
	Scale       int  `json:"scale"`
	Temperature int  `json:"temperature"`
	USBPowered  bool `json:"usbPowered"`
	ACPowered   bool `json:"acPowered"`
}
type Memory struct {
	Total     int64 `json:"total"`
	Free      int64 `json:"free"`
	Available int64 `json:"available"`
}
type CPU struct {
	Cores    int    `json:"cores"`
	Hardware string `json:"hardware"`
}
type Storage struct {
	Total     int64 `json:"total"`
	Available int64 `json:"available"`
}
type Info struct {
	UDID         string   `json:"udid,omitempty"`
	Version      string   `json:"version,omitempty"`
	Serial       string   `json:"serial,omitempty"`
	Brand        string   `json:"brand,omitempty"`
	Model        string   `json:"model,omitempty"`
	HWAddr       string   `json:"hwaddr,omitempty"`
	SDK          int      `json:"sdk,omitempty"`
	AgentVersion string   `json:"agentVersion,omitempty"`
	Display      *Display `json:"display,omitempty"`
	Density      *Density `json:"density,omitempty"`
	Battery      *Battery `json:"battery,omitempty"`
	Memory       *Memory  `json:"memory,omitempty"`
	CPU          *CPU     `json:"cpu,omitempty"`
	Arch         string   `json:"arch"`
	ABI          string   `json:"abi,omitempty"`
	Device       string   `json:"device"`
	BuildID      string   `json:"buildId"`
	DisplayID    string   `json:"displayId"`
	OpenGL       string   `json:"openGLVersion"`
	Locale       string   `json:"locale"`
	Manufacturer string   `json:"manufacturer"`
	Hardware     string   `json:"hardware"`
	HWVersion    string   `json:"hardwareVersion"`
	BootReason   string   `json:"bootReason"`
	Fingerprint  string   `json:"fingerPrint"`
	BuildDate    string   `json:"buildDate"`
	Storage      *Storage `json:"storage,omitempty"`
	Platform     string   `json:"platform"`
}
type Service struct {
	executor platform.Executor
	timeout  time.Duration
}

func New(executor platform.Executor, timeout time.Duration) *Service {
	return &Service{executor: executor, timeout: timeout}
}
func (s *Service) command(ctx context.Context, name string, args ...string) (string, error) {
	c, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.executor.Run(c, name, args...)
}
func (s *Service) property(ctx context.Context, name string) string {
	value, _ := s.command(ctx, "getprop", name)
	return strings.TrimSpace(value)
}
func (s *Service) Info(ctx context.Context) (Info, error) {
	serial, model := s.property(ctx, "ro.serialno"), s.property(ctx, "ro.product.model")
	hwaddr := s.property(ctx, "persist.sys.wifi.mac")
	sdk, _ := strconv.Atoi(s.property(ctx, "ro.build.version.sdk"))
	info := Info{
		UDID:    fmt.Sprintf("%s-%s-%s", serial, hwaddr, strings.ReplaceAll(model, " ", "_")),
		Version: s.property(ctx, "ro.build.version.release"), Serial: serial, Brand: s.property(ctx, "ro.product.brand"), Model: model, HWAddr: hwaddr, SDK: sdk,
		AgentVersion: buildinfo.Version, ABI: s.property(ctx, "ro.product.cpu.abi"), Arch: s.property(ctx, "ro.product.cpu.abilist"), Device: s.property(ctx, "ro.product.device"),
		BuildID: s.property(ctx, "ro.build.id"), DisplayID: s.property(ctx, "ro.build.display.id"), OpenGL: s.property(ctx, "ro.opengles.version"), Locale: s.property(ctx, "persist.sys.locale"),
		Manufacturer: s.property(ctx, "ro.product.manufacturer"), Hardware: s.property(ctx, "ro.boot.hardware"), HWVersion: s.property(ctx, "ro.boot.hwversion"),
		BootReason: s.property(ctx, "ro.boot.bootreason"), Fingerprint: s.property(ctx, "ro.bootimage.build.fingerprint"), BuildDate: s.property(ctx, "ro.bootimage.build.date"), Platform: "android",
	}
	info.Display, info.Density, info.Battery = s.display(ctx), s.density(ctx), s.battery(ctx)
	info.Memory, info.CPU, info.Storage = s.resources(ctx)
	return info, nil
}

var sizePattern = regexp.MustCompile(`(?m)(?:Physical|Override) size:\s*(\d+)x(\d+)`)
var densityPattern = regexp.MustCompile(`(?m)(?:Physical|Override) density:\s*(\d+)`)

func (s *Service) display(ctx context.Context) *Display {
	value, err := s.command(ctx, "wm", "size")
	if err != nil {
		return nil
	}
	matches := sizePattern.FindAllStringSubmatch(value, -1)
	if len(matches) == 0 {
		return nil
	}
	match := matches[len(matches)-1]
	width, _ := strconv.Atoi(match[1])
	height, _ := strconv.Atoi(match[2])
	return &Display{Width: width, Height: height}
}
func (s *Service) density(ctx context.Context) *Density {
	value, err := s.command(ctx, "wm", "density")
	if err != nil {
		return nil
	}
	matches := densityPattern.FindAllStringSubmatch(value, -1)
	if len(matches) == 0 {
		return nil
	}
	match := matches[len(matches)-1]
	parsed, _ := strconv.Atoi(match[1])
	return &Density{Density: parsed}
}
func (s *Service) battery(ctx context.Context) *Battery {
	value, err := s.command(ctx, "dumpsys", "battery")
	if err != nil {
		return nil
	}
	fields := colonFields(value)
	return &Battery{Level: atoi(fields["level"]), Scale: atoi(fields["scale"]), Temperature: atoi(fields["temperature"]), USBPowered: fields["USB powered"] == "true", ACPowered: fields["AC powered"] == "true"}
}
func (s *Service) resources(ctx context.Context) (*Memory, *CPU, *Storage) {
	var memory *Memory
	if value, err := s.command(ctx, "cat", "/proc/meminfo"); err == nil {
		fields := whitespaceFields(value)
		memory = &Memory{Total: fields["MemTotal"], Free: fields["MemFree"], Available: fields["MemAvailable"]}
	}
	var cpu *CPU
	if value, err := s.command(ctx, "cat", "/proc/cpuinfo"); err == nil {
		cores := strings.Count(value, "\nprocessor")
		if strings.HasPrefix(value, "processor") {
			cores++
		}
		if cores == 0 {
			cores = runtime.NumCPU()
		}
		cpu = &CPU{Cores: cores, Hardware: colonFields(value)["Hardware"]}
	}
	var storage *Storage
	if value, err := s.command(ctx, "df", "-k", "/data"); err == nil {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			lines := strings.Split(trimmed, "\n")
			fields := strings.Fields(lines[len(lines)-1])
			if len(fields) >= 4 {
				total, e1 := strconv.ParseInt(fields[1], 10, 64)
				available, e2 := strconv.ParseInt(fields[3], 10, 64)
				if e1 == nil && e2 == nil {
					storage = &Storage{Total: total * 1024, Available: available * 1024}
				}
			}
		}
	}
	return memory, cpu, storage
}
func colonFields(value string) map[string]string {
	result := map[string]string{}
	for _, line := range strings.Split(value, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), ":", 2)
		if len(parts) == 2 {
			result[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return result
}
func whitespaceFields(value string) map[string]int64 {
	result := map[string]int64{}
	for _, line := range strings.Split(value, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			parsed, _ := strconv.ParseInt(fields[1], 10, 64)
			result[strings.TrimSuffix(fields[0], ":")] = parsed
		}
	}
	return result
}
func atoi(value string) int { result, _ := strconv.Atoi(value); return result }

var foregroundPatterns = []*regexp.Regexp{
	regexp.MustCompile(`mResumedActivity[^\n]*? ([A-Za-z0-9._]+/[A-Za-z0-9._$]+)`),
	regexp.MustCompile(`mCurrentFocus=[^\n]*? ([A-Za-z0-9._]+/[A-Za-z0-9._$]+)`),
}
var activityForegroundPattern = regexp.MustCompile(`(?:topResumedActivity|mResumedActivity|ResumedActivity)[^\n]*ActivityRecord\{[^ ]+ [^ ]+ ([A-Za-z0-9._]+/[A-Za-z0-9._$]+)`)

func (s *Service) ForegroundPackage(ctx context.Context) (string, error) {
	component, err := s.ForegroundActivity(ctx)
	if err != nil {
		return "", err
	}
	if separator := strings.IndexByte(component, '/'); separator >= 0 {
		return component[:separator], nil
	}
	return "", nil
}

// ForegroundActivity returns a canonical package/class component. Relative
// Android class names are expanded so reports remain comparable across tools.
func (s *Service) ForegroundActivity(ctx context.Context) (string, error) {
	out, err := s.command(ctx, "dumpsys", "window", "windows")
	if err != nil {
		return "", err
	}
	for _, p := range foregroundPatterns {
		if m := p.FindStringSubmatch(out); len(m) == 2 {
			return canonicalComponent(m[1]), nil
		}
	}
	out, err = s.command(ctx, "dumpsys", "activity", "activities")
	if err != nil {
		return "", err
	}
	if match := activityForegroundPattern.FindStringSubmatch(out); len(match) == 2 {
		return canonicalComponent(match[1]), nil
	}
	return "", nil
}

func canonicalComponent(value string) string {
	parts := strings.SplitN(value, "/", 2)
	if len(parts) != 2 {
		return value
	}
	class := parts[1]
	if strings.HasPrefix(class, ".") {
		class = parts[0] + class
	}
	return parts[0] + "/" + class
}
func (s *Service) Wake(ctx context.Context) error {
	_, err := s.command(ctx, "input", "keyevent", "KEYCODE_WAKEUP")
	return err
}
func (s *Service) Shell(ctx context.Context, command string, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		timeout = s.timeout
	}
	c, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return s.executor.Run(c, "sh", "-c", command)
}
