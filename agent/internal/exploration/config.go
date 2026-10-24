package exploration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var packagePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(?:\.[A-Za-z][A-Za-z0-9_]*)+$`)
var requestPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

type Config struct {
	RequestID          string          `json:"requestId,omitempty"`
	Package            string          `json:"package"`
	MaxSteps           int             `json:"maxSteps"`
	DurationSeconds    int             `json:"durationSeconds,omitempty"`
	IntervalMillis     int             `json:"intervalMillis"`
	Seed               int64           `json:"seed,omitempty"`
	Execute            bool            `json:"execute"`
	EnableScroll       bool            `json:"enableScroll"`
	EnableBacktrack    bool            `json:"enableBacktrack"`
	RecoverPopups      bool            `json:"recoverPopups"`
	SpecialHandling    SpecialHandling `json:"specialHandling,omitempty"`
	MaxBacktracks      int             `json:"maxBacktracks"`
	ExpectedActivities []string        `json:"expectedActivities,omitempty"`
	Rules              Rules           `json:"rules,omitempty"`
	InputStrategy      InputStrategy   `json:"inputStrategy,omitempty"`
	FreshnessMode      string          `json:"freshnessMode,omitempty"`
	VersionCode        int64           `json:"versionCode,omitempty"`
	ActivityMode       string          `json:"activityMode,omitempty"`
	Activities         []string        `json:"activities,omitempty"`
	ControlBlacklist   []string        `json:"controlBlacklist,omitempty"`
	RenderFallbackMode string          `json:"renderFallbackMode,omitempty"`
	LowBatteryExit     bool            `json:"lowBatteryExit,omitempty"`
	MinBattery         int             `json:"minBatteryPercent,omitempty"`
	TargetCases        []TargetCase    `json:"targetCases,omitempty"`
}

type TargetCase struct {
	Activity string `json:"activity"`
	Task     string `json:"task,omitempty"`
	Case     string `json:"case"`
}

func (c *Config) normalize() error {
	c.Package = strings.TrimSpace(c.Package)
	if !packagePattern.MatchString(c.Package) {
		return errors.New("invalid Android package name")
	}
	if c.RequestID != "" && !requestPattern.MatchString(c.RequestID) {
		return errors.New("invalid requestId")
	}
	if c.RequestID == "" {
		c.RequestID = fmt.Sprintf("explore-%d", time.Now().UnixNano())
	}
	if c.DurationSeconds < 0 || c.DurationSeconds > 604800 {
		return errors.New("durationSeconds must be between 1 and 604800 when set")
	}
	if c.MaxSteps == 0 && c.DurationSeconds == 0 {
		c.MaxSteps = 100
	}
	if c.IntervalMillis == 0 {
		c.IntervalMillis = 250
	}
	if c.MaxBacktracks == 0 {
		c.MaxBacktracks = 20
	}
	if c.InputStrategy.CasesPerField == 0 {
		c.InputStrategy.CasesPerField = defaultInputCasesPerField
	}
	if c.InputStrategy.MaxLength == 0 {
		c.InputStrategy.MaxLength = defaultInputMaxLength
	}
	if c.DurationSeconds == 0 && (c.MaxSteps < 1 || c.MaxSteps > 10000) {
		return errors.New("maxSteps must be between 1 and 10000")
	}
	if c.IntervalMillis < 100 || c.IntervalMillis > 60000 {
		return errors.New("intervalMillis must be between 100 and 60000")
	}
	if c.MaxBacktracks < 1 || c.MaxBacktracks > 1000 {
		return errors.New("maxBacktracks must be between 1 and 1000")
	}
	if c.InputStrategy.CasesPerField < 1 || c.InputStrategy.CasesPerField > maxInputCasesPerField {
		return fmt.Errorf("inputStrategy.casesPerField must be between 1 and %d", maxInputCasesPerField)
	}
	if c.FreshnessMode == "" {
		c.FreshnessMode = freshnessFast
	}
	if c.FreshnessMode != freshnessFast && c.FreshnessMode != freshnessStrict {
		return errors.New("freshnessMode must be fast or strict")
	}
	if c.ActivityMode == "" {
		c.ActivityMode = "none"
	}
	if c.ActivityMode != "none" && c.ActivityMode != "allowlist" && c.ActivityMode != "blocklist" {
		return errors.New("activityMode must be none, allowlist, or blocklist")
	}
	if c.ActivityMode == "allowlist" && len(c.Activities) == 0 {
		return errors.New("allowlist requires at least one activity")
	}
	if c.RenderFallbackMode != "" && c.RenderFallbackMode != "off" && c.RenderFallbackMode != "bounded" && c.RenderFallbackMode != "continuous" {
		return errors.New("renderFallbackMode must be off, bounded, or continuous")
	}
	if c.LowBatteryExit && (c.MinBattery < 1 || c.MinBattery > 100) {
		return errors.New("minBatteryPercent must be between 1 and 100")
	}
	if len(c.Activities) > 64 || len(c.ControlBlacklist) > 64 || len(c.TargetCases) > 32 {
		return errors.New("session guard lists are limited")
	}
	if c.InputStrategy.MaxLength < 1 || c.InputStrategy.MaxLength > 256 {
		return errors.New("inputStrategy.maxLength must be between 1 and 256")
	}
	if len(c.Rules.DenyText) > 100 || len(c.Rules.AllowText) > 100 || len(c.Rules.AllowResourcePrefixes) > 100 || len(c.Rules.RecoveryText) > 100 {
		return errors.New("each rule list is limited to 100 entries")
	}
	if len(c.ExpectedActivities) > 500 {
		return errors.New("expectedActivities is limited to 500 entries")
	}
	if err := c.SpecialHandling.normalize(); err != nil {
		return err
	}
	seenActivities := map[string]struct{}{}
	cleanActivities := make([]string, 0, len(c.ExpectedActivities))
	for _, activity := range c.ExpectedActivities {
		activity = strings.TrimSpace(activity)
		if activity == "" || !strings.Contains(activity, "/") {
			return errors.New("expectedActivities entries must use package/class format")
		}
		if _, exists := seenActivities[activity]; !exists {
			seenActivities[activity] = struct{}{}
			cleanActivities = append(cleanActivities, activity)
		}
	}
	c.ExpectedActivities = cleanActivities
	return nil
}

// Validate checks a copy so scenario files can be verified without applying
// generated runtime values to the source document.
func (c *Config) Validate() error { return c.normalize() }

func configFingerprint(config Config) string {
	encoded, _ := json.Marshal(config)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func observationID(requestID string, sequence int, fingerprint string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%s", requestID, sequence, fingerprint)))
	return "obs-" + hex.EncodeToString(sum[:12])
}

func actionFingerprint(action Action) string {
	encoded, _ := json.Marshal(action)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func scopedFingerprint(fingerprint, activity string) string {
	sum := sha256.Sum256([]byte(activity + "\x00" + fingerprint))
	return fmt.Sprintf("%x", sum[:16])
}
