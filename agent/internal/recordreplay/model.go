package recordreplay

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const SchemaVersion = "xtest-recording/v1"

var packagePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(?:\.[A-Za-z][A-Za-z0-9_]*)+$`)
var requestPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)
var screenshotHashPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)
var referencePathPattern = regexp.MustCompile(`^screenshots/step-[0-9]{6}\.png$`)

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Bounds struct {
	Left   float64 `json:"left"`
	Top    float64 `json:"top"`
	Right  float64 `json:"right"`
	Bottom float64 `json:"bottom"`
}

func (b Bounds) valid() bool {
	return b.Left >= 0 && b.Top >= 0 && b.Right <= 1 && b.Bottom <= 1 && b.Right > b.Left && b.Bottom > b.Top
}

func (b Bounds) contains(point Point) bool {
	return point.X >= b.Left && point.X <= b.Right && point.Y >= b.Top && point.Y <= b.Bottom
}

type Contact struct {
	Index int   `json:"index"`
	Start Point `json:"start"`
	End   Point `json:"end"`
}

type Action struct {
	Type            string        `json:"type"`
	OffsetMillis    int64         `json:"offsetMillis"`
	DurationMillis  int64         `json:"durationMillis,omitempty"`
	Start           Point         `json:"start,omitempty"`
	End             Point         `json:"end,omitempty"`
	KeyCode         int           `json:"keyCode,omitempty"`
	Text            string        `json:"text,omitempty"`
	Focus           bool          `json:"focus,omitempty"`
	Contacts        []Contact     `json:"contacts,omitempty"`
	ScreenshotHash  string        `json:"screenshotHash,omitempty"`
	MaxHashDistance int           `json:"maxHashDistance,omitempty"`
	ReferencePath   string        `json:"referencePath,omitempty"`
	Target          *ActionTarget `json:"target,omitempty"`
}

type ActionTarget struct {
	ResourceID         string `json:"resourceId,omitempty"`
	Text               string `json:"text,omitempty"`
	ContentDescription string `json:"contentDescription,omitempty"`
	ClassName          string `json:"className,omitempty"`
	Activity           string `json:"activity,omitempty"`
	Bounds             Bounds `json:"bounds,omitempty"`
	ObservationID      string `json:"observationId,omitempty"`
}

type Case struct {
	SchemaVersion  string    `json:"schemaVersion"`
	Task           string    `json:"task,omitempty"`
	Name           string    `json:"name"`
	Package        string    `json:"package"`
	RecordedAt     time.Time `json:"recordedAt"`
	RecordedWidth  int       `json:"recordedWidth,omitempty"`
	RecordedHeight int       `json:"recordedHeight,omitempty"`
	Actions        []Action  `json:"actions"`
	Integrity      string    `json:"integrity"`
}

func (c *Case) Validate() error {
	if c.SchemaVersion != SchemaVersion {
		return errors.New("unsupported recording schemaVersion")
	}
	if !validLabel(c.Name) {
		return errors.New("recording name must be 1-80 safe characters")
	}
	if c.Task != "" && !validLabel(c.Task) {
		return errors.New("recording task must be 1-80 safe characters")
	}
	if !packagePattern.MatchString(strings.TrimSpace(c.Package)) {
		return errors.New("invalid Android package name")
	}
	if c.RecordedAt.IsZero() {
		return errors.New("recordedAt is required")
	}
	if (c.RecordedWidth == 0) != (c.RecordedHeight == 0) || c.RecordedWidth < 0 || c.RecordedHeight < 0 {
		return errors.New("recorded display dimensions must both be positive or both omitted")
	}
	if len(c.Actions) > 10000 {
		return errors.New("recording is limited to 10000 actions")
	}
	var previous int64 = -1
	for index, action := range c.Actions {
		if action.OffsetMillis < previous || action.OffsetMillis < 0 || action.OffsetMillis > int64((24*time.Hour)/time.Millisecond) {
			return fmt.Errorf("action %d has invalid timeline offset", index)
		}
		previous = action.OffsetMillis
		if action.DurationMillis < 0 || action.DurationMillis > 60000 {
			return fmt.Errorf("action %d has invalid duration", index)
		}
		if action.Target != nil {
			if !action.Target.Bounds.valid() || !safeTargetValue(action.Target.ResourceID) || !safeTargetValue(action.Target.Text) || !safeTargetValue(action.Target.ContentDescription) || !safeTargetValue(action.Target.ClassName) || !safeTargetValue(action.Target.Activity) || !safeTargetValue(action.Target.ObservationID) {
				return fmt.Errorf("action %d has invalid semantic target", index)
			}
		}
		switch action.Type {
		case "tap", "long_press", "double_tap":
			if !validPoint(action.Start) {
				return fmt.Errorf("action %d has invalid coordinates", index)
			}
		case "swipe":
			if !validPoint(action.Start) || !validPoint(action.End) {
				return fmt.Errorf("action %d has invalid coordinates", index)
			}
		case "key":
			if action.KeyCode != 4 {
				return fmt.Errorf("action %d uses a key outside the recording allowlist", index)
			}
		case "text":
			if !utf8.ValidString(action.Text) || len(action.Text) > 4096 || strings.ContainsRune(action.Text, '\x00') {
				return fmt.Errorf("action %d has invalid UTF-8 text", index)
			}
			if action.Focus && !validPoint(action.Start) {
				return fmt.Errorf("action %d has invalid focus coordinates", index)
			}
		case "multi_touch":
			if len(action.Contacts) < 2 || len(action.Contacts) > 10 || action.DurationMillis < 1 {
				return fmt.Errorf("action %d has invalid multi-touch contacts", index)
			}
			seen := map[int]bool{}
			for _, contact := range action.Contacts {
				if contact.Index < 0 || contact.Index > 9 || seen[contact.Index] || !validPoint(contact.Start) || !validPoint(contact.End) {
					return fmt.Errorf("action %d has invalid multi-touch contact", index)
				}
				seen[contact.Index] = true
			}
		case "assert_screenshot":
			if !screenshotHashPattern.MatchString(action.ScreenshotHash) || action.MaxHashDistance < 0 || action.MaxHashDistance > 16 {
				return fmt.Errorf("action %d has invalid screenshot assertion", index)
			}
			if action.ReferencePath != "" && !referencePathPattern.MatchString(action.ReferencePath) {
				return fmt.Errorf("action %d has invalid screenshot reference path", index)
			}
		default:
			return fmt.Errorf("action %d has unsupported type %q", index, action.Type)
		}
	}
	return nil
}

func safeTargetValue(value string) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= 256 && !strings.ContainsRune(value, '\x00')
}

func validLabel(value string) bool {
	value = strings.TrimSpace(value)
	count := utf8.RuneCountInString(value)
	if count < 1 || count > 80 || value == "." || value == ".." || strings.ContainsAny(value, "/\\\x00\r\n") {
		return false
	}
	for _, valueRune := range value {
		if valueRune < 0x20 || valueRune == 0x7f {
			return false
		}
	}
	return true
}

func (c *Case) Seal() error {
	if err := c.Validate(); err != nil {
		return err
	}
	digest, err := c.digest()
	if err != nil {
		return err
	}
	c.Integrity = "sha256:" + digest
	return nil
}

func (c *Case) Verify() error {
	if err := c.Validate(); err != nil {
		return err
	}
	digest, err := c.digest()
	if err != nil {
		return err
	}
	if c.Integrity != "sha256:"+digest {
		return errors.New("recording integrity check failed")
	}
	return nil
}

// Fingerprint returns the verified immutable case digest used by execution
// preflight. It deliberately re-verifies the case instead of trusting the
// serialized Integrity field.
func (c *Case) Fingerprint() (string, error) {
	if err := c.Verify(); err != nil {
		return "", err
	}
	return c.Integrity, nil
}

func (c *Case) digest() (string, error) {
	cloned := *c
	cloned.Integrity = ""
	cloned.RecordedAt = cloned.RecordedAt.UTC().Truncate(time.Millisecond)
	if cloned.Actions == nil {
		cloned.Actions = make([]Action, 0)
	}
	encoded, err := json.Marshal(cloned)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func validPoint(point Point) bool {
	return !math.IsNaN(point.X) && !math.IsNaN(point.Y) && !math.IsInf(point.X, 0) && !math.IsInf(point.Y, 0) && point.X >= 0 && point.X <= 1 && point.Y >= 0 && point.Y <= 1
}

func cloneCase(value Case) Case {
	value.Actions = append([]Action(nil), value.Actions...)
	for index := range value.Actions {
		value.Actions[index].Contacts = append([]Contact(nil), value.Actions[index].Contacts...)
	}
	return value
}
