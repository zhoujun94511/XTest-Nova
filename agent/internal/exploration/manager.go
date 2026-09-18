package exploration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/artifacts"
	"github.com/zhoujun94511/xtest-nova/agent/internal/companioncontrol"
	"github.com/zhoujun94511/xtest-nova/agent/internal/evidence"
	"github.com/zhoujun94511/xtest-nova/agent/internal/execution"
	"github.com/zhoujun94511/xtest-nova/agent/internal/platform"
)

var packagePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(?:\.[A-Za-z][A-Za-z0-9_]*)+$`)
var requestPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)
var errUnstableCapture = errors.New("foreground changed during hierarchy capture")
var ErrRequestConflict = errors.New("requestId was already used with different exploration configuration")

type HierarchySource interface {
	Hierarchy(context.Context) (string, error)
}

type ForegroundSource interface {
	ForegroundPackage(context.Context) (string, error)
}

type ActivitySource interface {
	ForegroundActivity(context.Context) (string, error)
}

type TextInjector interface {
	InjectText(context.Context, string) error
}

type Config struct {
	RequestID          string          `json:"requestId,omitempty"`
	Package            string          `json:"package"`
	MaxSteps           int             `json:"maxSteps"`
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
	if c.MaxSteps == 0 {
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
	if c.MaxSteps < 1 || c.MaxSteps > 10000 {
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

type Step struct {
	Number        int       `json:"number"`
	StepID        string    `json:"stepId"`
	ObservationID string    `json:"observationId"`
	At            time.Time `json:"at"`
	From          string    `json:"from"`
	Action        Action    `json:"action"`
	Destination   string    `json:"destination,omitempty"`
}

type State struct {
	RequestID            string             `json:"requestId,omitempty"`
	Identity             execution.Identity `json:"identity"`
	Running              bool               `json:"running"`
	Stopping             bool               `json:"stopping,omitempty"`
	Finalizing           bool               `json:"finalizing,omitempty"`
	Execute              bool               `json:"execute"`
	EnableScroll         bool               `json:"enableScroll"`
	EnableBacktrack      bool               `json:"enableBacktrack"`
	RecoverPopups        bool               `json:"recoverPopups"`
	MaxBacktracks        int                `json:"maxBacktracks,omitempty"`
	Package              string             `json:"package,omitempty"`
	Seed                 int64              `json:"seed,omitempty"`
	StartedAt            *time.Time         `json:"startedAt,omitempty"`
	EndedAt              *time.Time         `json:"endedAt,omitempty"`
	Steps                int                `json:"steps"`
	DiscoveredStates     int                `json:"discoveredStates"`
	DiscoveredEdges      int                `json:"discoveredEdges"`
	Backtracks           int                `json:"backtracks"`
	CycleDetections      int                `json:"cycleDetections"`
	BlockedEdges         int                `json:"blockedEdges"`
	UnstableCaptures     int                `json:"unstableCaptures"`
	StaleActions         int                `json:"staleActions"`
	EffectiveActions     int                `json:"effectiveActions"`
	IneffectiveActions   int                `json:"ineffectiveActions"`
	ExternalActions      int                `json:"externalActions"`
	LastCycle            *CycleEvent        `json:"lastCycle,omitempty"`
	Scrolls              int                `json:"scrolls"`
	ScrollAttempts       int                `json:"scrollAttempts"`
	ScrollProgress       int                `json:"scrollProgress"`
	ScrollStalls         int                `json:"scrollStalls"`
	Inputs               int                `json:"inputs"`
	InputFields          int                `json:"inputFields"`
	Recoveries           int                `json:"recoveries"`
	SpecialActions       int                `json:"specialActions"`
	AdEncounters         int                `json:"adEncounters"`
	AdCloseActions       int                `json:"adCloseActions"`
	AdBackFallbacks      int                `json:"adBackFallbacks"`
	AdExternalRecoveries int                `json:"adExternalRecoveries"`
	AdRecoveries         int                `json:"adRecoveries"`
	AdWaitMillis         int64              `json:"adWaitMillis"`
	LastAdType           string             `json:"lastAdType,omitempty"`
	LastAdPhase          string             `json:"lastAdPhase,omitempty"`
	OverlaySuppressed    bool               `json:"overlaySuppressed"`
	OverlayControlError  string             `json:"overlayControlError,omitempty"`
	LastSpecial          *SpecialEvent      `json:"lastSpecial,omitempty"`
	ObservedActivities   int                `json:"observedActivities"`
	ActivityCovered      int                `json:"activityCovered"`
	ActivityTotal        int                `json:"activityTotal,omitempty"`
	ActivityCoverage     *float64           `json:"activityCoveragePercent,omitempty"`
	CurrentFingerprint   string             `json:"currentFingerprint,omitempty"`
	CurrentObservationID string             `json:"currentObservationId,omitempty"`
	LastAction           *Action            `json:"lastAction,omitempty"`
	StopReason           string             `json:"stopReason,omitempty"`
	Error                string             `json:"error,omitempty"`
	ArtifactDir          string             `json:"artifactDir,omitempty"`
	EvidenceIndexPath    string             `json:"evidenceIndexPath,omitempty"`
}

type CycleEvent struct {
	At          time.Time `json:"at"`
	Period      int       `json:"period,omitempty"`
	Repetitions int       `json:"repetitions"`
	From        string    `json:"from"`
	Action      string    `json:"action"`
	To          string    `json:"to"`
	Reason      string    `json:"reason"`
}

type Graph struct {
	States []GraphState `json:"states"`
	Edges  []GraphEdge  `json:"edges"`
}

type GraphState struct {
	Fingerprint string `json:"fingerprint"`
	Activity    string `json:"activity,omitempty"`
	Parent      string `json:"parent,omitempty"`
	NodeCount   int    `json:"nodeCount"`
	ActionCount int    `json:"actionCount"`
}

type GraphEdge struct {
	From     string `json:"from"`
	ActionID string `json:"actionId"`
	To       string `json:"to"`
}

type graphNode struct {
	analysis            Analysis
	activity            string
	semantic            string
	tried               map[string]bool
	actionAttempts      map[string]int
	nonScrollSelections int
}

const (
	minimumEmptyCapturesBeforeExhaustion = 3
	minimumExhaustedCapturesBeforeStop   = 2
	maximumConsecutiveUnstableCaptures   = 20
	transitionSettleDuration             = 1500 * time.Millisecond
	initialAsyncSettleDuration           = 8 * time.Second
)

type Manager struct {
	hierarchy            HierarchySource
	foreground           ForegroundSource
	executor             platform.Executor
	textInjector         TextInjector
	mu                   sync.Mutex
	state                State
	config               Config
	requestFingerprint   string
	nodes                map[string]*graphNode
	edges                []GraphEdge
	steps                []Step
	parents              map[string]string
	returns              map[string]Action
	returnAttempts       map[string]int
	blockedEdges         map[string]bool
	seenSemantic         map[string]bool
	cycles               cycleGuard
	activities           map[string]struct{}
	inputTried           map[string]bool
	inputFields          map[string]bool
	stopRequested        bool
	root                 string
	cancel               context.CancelFunc
	done                 chan struct{}
	coordinator          *execution.Coordinator
	identity             execution.Identity
	receipts             *execution.ReceiptStore
	artifactRoot         string
	persistArtifactsHook func(Config)
}

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

func New(hierarchy HierarchySource, foreground ForegroundSource, executor platform.Executor, injectors ...TextInjector) *Manager {
	var injector TextInjector
	if len(injectors) > 0 {
		injector = injectors[0]
	}
	return &Manager{hierarchy: hierarchy, foreground: foreground, executor: executor, textInjector: injector, nodes: map[string]*graphNode{}, parents: map[string]string{}, returns: map[string]Action{}, returnAttempts: map[string]int{}, blockedEdges: map[string]bool{}, seenSemantic: map[string]bool{}, activities: map[string]struct{}{}, inputTried: map[string]bool{}, inputFields: map[string]bool{}, receipts: execution.NewReceiptStore()}
}

func canControlCompanionOverlay(executor platform.Executor) bool {
	switch executor.(type) {
	case platform.OSExecutor, *platform.OSExecutor:
		return true
	default:
		return false
	}
}

func (m *Manager) SetExecutionCoordinator(value *execution.Coordinator) {
	m.mu.Lock()
	m.coordinator = value
	m.mu.Unlock()
}

func (m *Manager) SetArtifactRoot(value string) {
	m.mu.Lock()
	m.artifactRoot = value
	m.mu.Unlock()
}

func (m *Manager) Receipts() []execution.ActionReceipt {
	m.mu.Lock()
	store := m.receipts
	m.mu.Unlock()
	if store == nil {
		return nil
	}
	return store.All()
}

func (m *Manager) Preview(ctx context.Context, targetPackage string, rules Rules) (Analysis, error) {
	targetPackage = strings.TrimSpace(targetPackage)
	if !packagePattern.MatchString(targetPackage) {
		return Analysis{}, errors.New("invalid Android package name")
	}
	foreground, activity, hierarchy, err := m.capture(ctx)
	if err != nil {
		return Analysis{}, err
	}
	if foreground != targetPackage {
		return Analysis{}, fmt.Errorf("target package is not foreground: %s", foreground)
	}
	analysis, err := Analyze(hierarchy, targetPackage, rules)
	if err != nil {
		return Analysis{}, err
	}
	analysis.Activity = activity
	analysis.Fingerprint = scopedFingerprint(analysis.Fingerprint, activity)
	analysis.ObservationID = observationID("preview", 1, analysis.Fingerprint)
	return analysis, nil
}

func (m *Manager) capture(ctx context.Context) (foreground, activity, hierarchy string, err error) {
	foreground, activity, err = m.currentForeground(ctx)
	if err != nil {
		return "", "", "", err
	}
	hierarchy, err = m.hierarchy.Hierarchy(ctx)
	if err != nil {
		return "", "", "", err
	}
	afterForeground, afterActivity, err := m.currentForeground(ctx)
	if err != nil {
		return "", "", "", err
	}
	if foreground != afterForeground || activity != afterActivity {
		return "", "", "", fmt.Errorf("%w: %s -> %s", errUnstableCapture, activity, afterActivity)
	}
	return foreground, activity, hierarchy, nil
}

func (m *Manager) currentForeground(ctx context.Context) (foreground, activity string, err error) {
	if source, ok := m.foreground.(ActivitySource); ok {
		activity, err = source.ForegroundActivity(ctx)
		if separator := strings.IndexByte(activity, '/'); separator >= 0 {
			foreground = activity[:separator]
		}
	} else {
		foreground, err = m.foreground.ForegroundPackage(ctx)
	}
	if err != nil {
		return "", "", err
	}
	return foreground, activity, nil
}

func (m *Manager) Start(config Config) (State, error) {
	if err := config.normalize(); err != nil {
		return m.State(), err
	}
	if !config.Execute {
		return m.State(), errors.New("execute=true is required; use preview for read-only analysis")
	}
	fingerprint := configFingerprint(config)
	m.mu.Lock()
	if m.state.Running || m.state.Stopping || m.state.Finalizing {
		state := m.state
		m.mu.Unlock()
		if state.RequestID == config.RequestID {
			if fingerprint != m.requestFingerprint {
				return state, ErrRequestConflict
			}
			return state, nil
		}
		return state, errors.New("exploration session already active")
	}
	if m.state.RequestID == config.RequestID && m.state.StartedAt != nil {
		state := cloneState(m.state)
		m.mu.Unlock()
		if fingerprint != m.requestFingerprint {
			return state, ErrRequestConflict
		}
		return state, nil
	}
	var identity execution.Identity
	if m.coordinator != nil {
		var acquireErr error
		identity, acquireErr = m.coordinator.Acquire("exploration", config.RequestID)
		if acquireErr != nil {
			m.mu.Unlock()
			return m.State(), acquireErr
		}
	} else {
		identity = execution.NewIdentity("exploration", config.RequestID, uint64(time.Now().UnixNano()))
	}
	ctx, cancel := context.WithCancel(context.Background())
	now := time.Now().UTC()
	m.config = config
	m.requestFingerprint = fingerprint
	m.nodes = map[string]*graphNode{}
	m.edges = nil
	m.steps = nil
	m.parents = map[string]string{}
	m.returns = map[string]Action{}
	m.returnAttempts = map[string]int{}
	m.blockedEdges = map[string]bool{}
	m.seenSemantic = map[string]bool{}
	m.cycles = cycleGuard{}
	m.activities = map[string]struct{}{}
	m.inputTried = map[string]bool{}
	m.inputFields = map[string]bool{}
	m.stopRequested = false
	m.root = ""
	m.cancel = cancel
	m.done = make(chan struct{})
	m.identity = identity
	m.receipts = execution.NewReceiptStore()
	m.state = State{
		RequestID: config.RequestID, Identity: identity, Running: true, Execute: true, Package: config.Package, Seed: config.Seed, StartedAt: &now,
		EnableScroll: config.EnableScroll, EnableBacktrack: config.EnableBacktrack,
		RecoverPopups: config.RecoverPopups, MaxBacktracks: config.MaxBacktracks,
	}
	state := m.state
	m.mu.Unlock()
	go m.run(ctx, config, m.done)
	return state, nil
}

func (m *Manager) run(ctx context.Context, config Config, done chan struct{}) {
	defer func() {
		m.mu.Lock()
		if m.done == done {
			if m.state.Running {
				m.finishLocked("stopped", "")
			}
			m.cancel = nil
		}
		persistHook := m.persistArtifactsHook
		identity, coordinator := m.identity, m.coordinator
		m.mu.Unlock()
		if persistHook != nil {
			persistHook(config)
		} else {
			m.persistArtifacts(config)
		}
		if coordinator != nil {
			coordinator.Release(identity)
		}
		m.mu.Lock()
		if m.done == done {
			m.state.Finalizing = false
		}
		m.mu.Unlock()
		close(done)
	}()
	var overlayErr error
	overlaySupported := canControlCompanionOverlay(m.executor)
	if overlaySupported {
		overlayContext, cancelOverlay := context.WithTimeout(ctx, 3*time.Second)
		overlayErr = companioncontrol.SuppressOverlay(overlayContext, m.executor)
		cancelOverlay()
	}
	m.mu.Lock()
	if overlaySupported && overlayErr == nil {
		m.state.OverlaySuppressed = true
	} else if overlayErr != nil {
		m.state.OverlayControlError = overlayErr.Error()
	}
	m.mu.Unlock()
	if overlaySupported && overlayErr == nil {
		defer func() {
			restoreContext, cancelRestore := context.WithTimeout(context.Background(), 3*time.Second)
			restoreErr := companioncontrol.RestoreOverlay(restoreContext, m.executor)
			cancelRestore()
			m.mu.Lock()
			m.state.OverlaySuppressed = false
			if restoreErr != nil {
				m.state.OverlayControlError = restoreErr.Error()
			}
			m.mu.Unlock()
		}()
	}
	pending := -1
	specialAttempts := map[string]int{}
	specialCooldownUntil := map[string]time.Time{}
	specialWaitStarted := map[string]time.Time{}
	externalAttempts := map[string]int{}
	var settleUntil time.Time
	settleForeground := ""
	emptyCaptures := 0
	exhaustedFingerprint := ""
	exhaustedCaptures := 0
	consecutiveUnstableCaptures := 0
	actionAttemptNumber := 0
	initialSettleUntil := time.Now().Add(initialAsyncSettleDuration)
	adActive := false
	adEncounterSequence := 0
	var adStartedAt time.Time
	defer func() {
		if adActive && !adStartedAt.IsZero() {
			m.mu.Lock()
			m.state.AdWaitMillis += time.Since(adStartedAt).Milliseconds()
			m.mu.Unlock()
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		foreground, activity, hierarchy, err := m.capture(ctx)
		if err != nil {
			if errors.Is(err, errUnstableCapture) {
				consecutiveUnstableCaptures++
				m.mu.Lock()
				m.state.UnstableCaptures++
				m.mu.Unlock()
				if consecutiveUnstableCaptures >= maximumConsecutiveUnstableCaptures {
					m.finish("unstable_capture_limit", fmt.Sprintf("foreground did not stabilize after %d consecutive captures", consecutiveUnstableCaptures))
					return
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Duration(config.IntervalMillis) * time.Millisecond):
				}
				continue
			}
			m.finish("safety_stop", err.Error())
			return
		}
		consecutiveUnstableCaptures = 0
		if foreground == config.Package {
			clear(externalAttempts)
		}
		special, specialFound, err := inspectSpecialActivity(hierarchy, config.Package, foreground, activity, config.SpecialHandling)
		if err != nil {
			m.finish("safety_stop", err.Error())
			return
		}
		if specialFound && special.Kind == "ad" {
			m.mu.Lock()
			if !adActive {
				adActive, adStartedAt = true, time.Now()
				adEncounterSequence++
				m.state.AdEncounters++
			}
			m.state.LastAdType, m.state.LastAdPhase = special.AdType, special.AdPhase
			m.mu.Unlock()
		} else if adActive && foreground == config.Package {
			m.mu.Lock()
			m.state.AdRecoveries++
			m.state.AdWaitMillis += time.Since(adStartedAt).Milliseconds()
			m.mu.Unlock()
			adActive = false
		}
		if specialFound {
			m.mu.Lock()
			attemptKey := special.Kind + "|" + special.Fingerprint
			if special.Kind == "ad" {
				// Full-screen ad WebViews animate and mutate text while the same
				// interruption remains active. Bind the budget to the encounter,
				// otherwise each new page fingerprint could reset the attempt cap.
				attemptKey = fmt.Sprintf("ad|%d|%s", adEncounterSequence, activity)
			}
			if special.Outcome == "wait" {
				started, exists := specialWaitStarted[attemptKey]
				if !exists {
					started = time.Now()
					specialWaitStarted[attemptKey] = started
				}
				waitLimit := special.WaitLimitSeconds
				if waitLimit <= 0 {
					waitLimit = config.SpecialHandling.AdMaxWaitSeconds
				}
				if time.Since(started) < time.Duration(waitLimit)*time.Second {
					last := special
					m.state.LastSpecial = &last
					m.mu.Unlock()
					if err := waitAction(ctx, time.Duration(min(config.IntervalMillis, 1000))*time.Millisecond); err != nil {
						return
					}
					continue
				}
				delete(specialWaitStarted, attemptKey)
				if special.FallbackAction != nil {
					fallback := *special.FallbackAction
					special.Action = &fallback
					special.Outcome = "fallback_action"
				}
			} else {
				delete(specialWaitStarted, attemptKey)
			}
			if deadline := specialCooldownUntil[attemptKey]; special.Kind == "ad" && special.Outcome != "explore" && time.Now().Before(deadline) {
				m.mu.Unlock()
				if err := waitAction(ctx, time.Duration(min(config.IntervalMillis, 1000))*time.Millisecond); err != nil {
					return
				}
				continue
			}
			if specialAttempts[attemptKey] > 0 && special.FallbackAction != nil {
				fallback := *special.FallbackAction
				special.Action = &fallback
				special.Outcome = "fallback_action"
			}
			last := special
			m.state.LastSpecial = &last
			if special.Outcome == "explore" {
				m.mu.Unlock()
			} else {
				if (special.Outcome != "action" && special.Outcome != "fallback_action") || special.Action == nil {
					reason := "special_unhandled"
					if special.Outcome == "policy_required" {
						reason = "policy_required"
					}
					m.finishLocked(reason, special.Reason)
					m.mu.Unlock()
					return
				}
				if m.state.Steps >= config.MaxSteps {
					m.finishLocked("max_steps", "")
					m.mu.Unlock()
					return
				}
				if specialAttempts[attemptKey] >= config.SpecialHandling.MaxAttempts {
					last.Outcome = "attempt_limit"
					last.Reason = "special scene did not change after bounded recovery attempts"
					m.state.LastSpecial = &last
					m.finishLocked("special_handling_failed", last.Reason)
					m.mu.Unlock()
					return
				}
				specialAttempts[attemptKey]++
				if special.Kind == "ad" {
					specialCooldownUntil[attemptKey] = time.Now().Add(2 * time.Second)
				}
				actionAttemptNumber++
				stepNumber := actionAttemptNumber
				stepID := fmt.Sprintf("%s:step-%06d", config.RequestID, stepNumber)
				observation := observationID(config.RequestID, stepNumber, special.Fingerprint)
				actionDigest := actionFingerprint(*special.Action)
				m.mu.Unlock()

				freshFingerprint, freshErr := m.validateSpecialObservation(ctx, config, special, activity)
				receipt, duplicate, receiptErr := m.receipts.Accept(execution.ActionReceipt{StepID: stepID, ObservationID: observation, SourceFingerprint: special.Fingerprint, ActionFingerprint: actionDigest})
				if receiptErr != nil {
					m.finish("safety_stop", receiptErr.Error())
					return
				}
				if duplicate {
					if receipt.Status == execution.ReceiptFailed || receipt.Status == execution.ReceiptRejected {
						m.finish("input_failed", receipt.Error)
						return
					}
					continue
				}
				if freshErr != nil {
					message := "special scene changed before action execution"
					if freshErr != nil {
						message = freshErr.Error()
					}
					m.receipts.Finish(stepID, execution.ReceiptRejected, "stale_observation", message, "", freshFingerprint)
					m.mu.Lock()
					m.state.StaleActions++
					specialAttempts[attemptKey]--
					m.mu.Unlock()
					continue
				}
				commandCtx, cancel := context.WithTimeout(ctx, actionTimeout(*special.Action))
				err = m.executeAction(commandCtx, *special.Action)
				cancel()
				if err != nil {
					m.receipts.Finish(stepID, execution.ReceiptFailed, "input_failed", err.Error(), "", "")
					m.finish("input_failed", err.Error())
					return
				}
				m.receipts.Finish(stepID, execution.ReceiptExecuted, "", "", "", "")
				settleUntil = time.Now().Add(transitionSettleDuration)
				settleForeground = special.Foreground
				emptyCaptures = 0
				exhaustedFingerprint = ""
				exhaustedCaptures = 0
				m.mu.Lock()
				step := Step{Number: stepNumber, StepID: stepID, ObservationID: observation, At: time.Now().UTC(), From: special.Fingerprint, Action: *special.Action}
				m.steps = append(m.steps, step)
				m.state.Steps = len(m.steps)
				m.state.Recoveries++
				m.state.SpecialActions++
				if special.Kind == "ad" {
					if special.Action.Type == "back" {
						m.state.AdBackFallbacks++
					} else {
						m.state.AdCloseActions++
					}
				}
				lastAction := *special.Action
				m.state.LastAction = &lastAction
				m.refreshCountsLocked()
				m.mu.Unlock()
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Duration(config.IntervalMillis) * time.Millisecond):
				}
				continue
			}
		}
		if foreground != config.Package && time.Now().Before(settleUntil) && (isTransientController(foreground) || foreground == settleForeground) {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Duration(config.IntervalMillis) * time.Millisecond):
			}
			continue
		}
		if foreground != config.Package && pending >= 0 && m.steps[pending].Action.Backtrack {
			commandCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			_, launchErr := m.executor.Run(commandCtx, "sh", "/system/bin/monkey", "-p", config.Package, "-c", "android.intent.category.LAUNCHER", "1")
			cancel()
			if launchErr != nil {
				m.finish("safety_stop", fmt.Sprintf("restore target after backtrack: %v", launchErr))
				return
			}
			pending = -1
			emptyCaptures = 0
			exhaustedFingerprint = ""
			exhaustedCaptures = 0
			settleUntil = time.Now().Add(transitionSettleDuration)
			settleForeground = ""
			m.mu.Lock()
			m.state.Recoveries++
			m.mu.Unlock()
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Duration(config.IntervalMillis) * time.Millisecond):
			}
			continue
		}
		if foreground != config.Package {
			const maxExternalRecoveries = 3
			if externalAttempts[foreground] >= maxExternalRecoveries {
				m.finish("safety_stop", fmt.Sprintf("target package is not foreground after %d recovery attempts: %s", maxExternalRecoveries, foreground))
				return
			}
			externalAttempts[foreground]++
			m.mu.Lock()
			if pending >= 0 {
				pendingStep := &m.steps[pending]
				if source := m.nodes[pendingStep.From]; source != nil {
					m.blockEdgeLocked(source.semantic, semanticActionKey(pendingStep.Action))
				}
				pendingStep.Destination = "external:" + foreground
				m.state.ExternalActions++
			}
			m.state.Recoveries++
			if adActive {
				m.state.AdExternalRecoveries++
			}
			m.mu.Unlock()
			commandCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			_, backErr := m.executor.Run(commandCtx, "input", "keyevent", "BACK")
			cancel()
			if backErr != nil {
				m.finish("safety_stop", fmt.Sprintf("dismiss external package %s: %v", foreground, backErr))
				return
			}
			commandCtx, cancel = context.WithTimeout(ctx, 5*time.Second)
			_, launchErr := m.executor.Run(commandCtx, "sh", "/system/bin/monkey", "-p", config.Package, "-c", "android.intent.category.LAUNCHER", "1")
			cancel()
			if launchErr != nil {
				m.finish("safety_stop", fmt.Sprintf("restore target from external package %s: %v", foreground, launchErr))
				return
			}
			pending = -1
			emptyCaptures = 0
			exhaustedFingerprint = ""
			exhaustedCaptures = 0
			settleUntil = time.Now().Add(transitionSettleDuration)
			settleForeground = ""
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Duration(config.IntervalMillis) * time.Millisecond):
			}
			continue
		}
		// Special-scene actions and controlled relaunches often land on a target
		// activity before its asynchronous content is complete. Continue to
		// inspect special/system scenes above, but do not build the ordinary graph
		// from a partially populated target hierarchy during the settle window.
		if time.Now().Before(settleUntil) {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Duration(config.IntervalMillis) * time.Millisecond):
			}
			continue
		}
		analysisRules := config.Rules
		analysis, err := AnalyzeWithInputStrategy(hierarchy, config.Package, analysisRules, config.Seed, config.InputStrategy)
		if err != nil {
			m.finish("safety_stop", err.Error())
			return
		}
		analysis.Activity = activity
		analysis.Fingerprint = scopedFingerprint(analysis.Fingerprint, activity)
		if len(analysis.Actions) == 0 {
			emptyCaptures++
		}
		if len(analysis.Actions) == 0 && (time.Now().Before(initialSettleUntil) && isAsyncStartupActivity(activity) || time.Now().Before(settleUntil) || emptyCaptures < minimumEmptyCapturesBeforeExhaustion) {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Duration(config.IntervalMillis) * time.Millisecond):
			}
			continue
		}
		if len(analysis.Actions) > 0 {
			emptyCaptures = 0
		}
		m.mu.Lock()
		analysis.ObservationID = observationID(config.RequestID, len(m.steps)+1, analysis.Fingerprint)
		node := m.nodes[analysis.Fingerprint]
		newState := node == nil
		currentSemantic := semanticStateKey(analysis, activity)
		if !m.seenSemantic[currentSemantic] {
			if len(m.seenSemantic) > 0 {
				m.cycles.resetProgressWindow()
			}
			m.seenSemantic[currentSemantic] = true
		}
		if node == nil {
			node = &graphNode{analysis: analysis, activity: activity, semantic: currentSemantic, tried: map[string]bool{}, actionAttempts: map[string]int{}}
			m.nodes[analysis.Fingerprint] = node
		}
		for _, candidate := range analysis.Actions {
			if candidate.Type == "input" {
				m.inputFields[inputFieldKey(activity, candidate)] = true
			}
		}
		if activity != "" {
			m.activities[activity] = struct{}{}
			if node.activity == "" {
				node.activity = activity
			}
		}
		if pending >= 0 {
			pendingStep := &m.steps[pending]
			pendingStep.Destination = analysis.Fingerprint
			m.receipts.Finish(pendingStep.StepID, execution.ReceiptExecuted, "", "", observationID(config.RequestID, pendingStep.Number+1, analysis.Fingerprint), analysis.Fingerprint)
			m.edges = append(m.edges, GraphEdge{From: pendingStep.From, ActionID: pendingStep.Action.ID, To: analysis.Fingerprint})
			if pendingStep.Action.Type == "swipe" && !pendingStep.Action.Backtrack {
				if pendingStep.From == analysis.Fingerprint {
					m.state.ScrollStalls++
				} else {
					m.state.ScrollProgress++
				}
			}
			fromSemantic := pendingStep.From
			if fromNode := m.nodes[pendingStep.From]; fromNode != nil {
				fromSemantic = fromNode.semantic
			}
			if fromSemantic == currentSemantic {
				m.state.IneffectiveActions++
			} else {
				m.state.EffectiveActions++
			}
			actionKey := semanticActionKey(pendingStep.Action)
			if pendingStep.Action.Backtrack {
				actionKey = "backtrack"
				expected := m.parents[pendingStep.From]
				if expected == "" || expected != analysis.Fingerprint {
					m.blockEdgeLocked(fromSemantic, actionKey)
				}
			}
			cycleFrom, cycleTo := fromSemantic, currentSemantic
			fromActivity := ""
			if fromNode := m.nodes[pendingStep.From]; fromNode != nil {
				fromActivity = fromNode.activity
			}
			if fromActivity != "" && activity != "" && fromActivity != activity {
				cycleFrom, cycleTo = "activity:"+fromActivity, "activity:"+activity
			}
			if period := observeExplorationCycle(&m.cycles, pendingStep.Action, cycleFrom, actionKey, cycleTo, fromSemantic); period > 0 {
				m.state.CycleDetections++
				reportedPeriod, reason := period, "repeated_cycle"
				if period > cycleMaxPeriod {
					reportedPeriod, reason = 0, "repeated_transition"
				}
				m.state.LastCycle = &CycleEvent{At: time.Now().UTC(), Period: reportedPeriod, Repetitions: cycleRepetitions, From: cycleFrom, Action: actionKey, To: cycleTo, Reason: reason}
				if period <= cycleMaxPeriod && len(m.cycles.history) >= period {
					for _, sample := range m.cycles.history[len(m.cycles.history)-period:] {
						m.blockEdgeLocked(sample.blockFrom, sample.action)
					}
				} else {
					m.blockEdgeLocked(fromSemantic, actionKey)
				}
			}
			if !pendingStep.Action.Backtrack && newState && pendingStep.From != analysis.Fingerprint {
				m.parents[analysis.Fingerprint] = pendingStep.From
				m.returns[analysis.Fingerprint] = inverseAction(pendingStep.Action, analysis.Fingerprint, pendingStep.From)
			}
			pending = -1
		}
		if m.root == "" {
			m.root = analysis.Fingerprint
		}
		m.state.CurrentFingerprint = analysis.Fingerprint
		m.state.CurrentObservationID = analysis.ObservationID
		m.refreshCountsLocked()
		if m.state.Steps >= config.MaxSteps {
			m.finishLocked("max_steps", "")
			m.mu.Unlock()
			return
		}
		action, ok := selectActionBlocked(node, config.Seed, config.EnableScroll, config.RecoverPopups, func(action Action) bool {
			return m.blockedEdges[explorationEdgeKey(node.semantic, semanticActionKey(action))] || (action.Type == "input" && m.inputTried[inputExecutionKey(activity, action)])
		})
		if !ok {
			returnAction, hasReturn := m.returns[analysis.Fingerprint]
			returnBlocked := m.blockedEdges[explorationEdgeKey(node.semantic, "backtrack")]
			if config.EnableBacktrack && hasReturn && !returnBlocked && m.returnAttempts[analysis.Fingerprint] == 0 && m.state.Backtracks < config.MaxBacktracks {
				action = returnAction
				ok = true
				m.returnAttempts[analysis.Fingerprint]++
			} else {
				if exhaustedFingerprint != analysis.Fingerprint {
					exhaustedFingerprint = analysis.Fingerprint
					exhaustedCaptures = 1
				} else {
					exhaustedCaptures++
				}
				if exhaustedCaptures < minimumExhaustedCapturesBeforeStop {
					m.mu.Unlock()
					select {
					case <-ctx.Done():
						return
					case <-time.After(time.Duration(config.IntervalMillis) * time.Millisecond):
					}
					continue
				}
				reason := "state_exhausted"
				if analysis.Fingerprint == m.root {
					reason = "graph_exhausted"
				}
				m.finishLocked(reason, "")
				m.mu.Unlock()
				return
			}
		}
		exhaustedFingerprint = ""
		exhaustedCaptures = 0
		actionAttemptNumber++
		stepNumber := actionAttemptNumber
		stepID := fmt.Sprintf("%s:step-%06d", config.RequestID, stepNumber)
		observation := observationID(config.RequestID, stepNumber, analysis.Fingerprint)
		actionDigest := actionFingerprint(action)
		m.mu.Unlock()

		freshFingerprint, freshErr := m.validateObservation(ctx, config, activity, analysis.Fingerprint, analysisRules)
		receipt, duplicate, receiptErr := m.receipts.Accept(execution.ActionReceipt{StepID: stepID, ObservationID: observation, SourceFingerprint: analysis.Fingerprint, ActionFingerprint: actionDigest})
		if receiptErr != nil {
			m.finish("safety_stop", receiptErr.Error())
			return
		}
		if duplicate { // A retried step is observable but never executes twice.
			if receipt.Status == execution.ReceiptFailed || receipt.Status == execution.ReceiptRejected {
				m.finish("input_failed", receipt.Error)
				return
			}
			continue
		}
		if freshErr != nil || freshFingerprint != analysis.Fingerprint {
			message := "page changed before action execution"
			if freshErr != nil {
				message = freshErr.Error()
			}
			m.receipts.Finish(stepID, execution.ReceiptRejected, "stale_observation", message, "", freshFingerprint)
			m.mu.Lock()
			m.state.StaleActions++
			m.mu.Unlock()
			continue
		}
		commandCtx, cancel := context.WithTimeout(ctx, actionTimeout(action))
		err = m.executeAction(commandCtx, action)
		cancel()
		if err != nil {
			m.receipts.Finish(stepID, execution.ReceiptFailed, "input_failed", err.Error(), "", "")
			m.finish("input_failed", err.Error())
			return
		}
		m.receipts.Finish(stepID, execution.ReceiptExecuted, "", "", "", "")
		m.mu.Lock()
		if !action.Backtrack {
			node.actionAttempts[action.ID]++
			if action.Type != "swipe" || node.actionAttempts[action.ID] >= maxStableScrollAttempts {
				node.tried[action.ID] = true
			}
			if action.Type == "swipe" {
				node.nonScrollSelections = 0
			} else {
				node.nonScrollSelections++
			}
			if action.Type == "input" {
				m.inputTried[inputExecutionKey(activity, action)] = true
			}
		}
		step := Step{Number: stepNumber, StepID: stepID, ObservationID: observation, At: time.Now().UTC(), From: analysis.Fingerprint, Action: action}
		m.steps = append(m.steps, step)
		pending = len(m.steps) - 1
		m.state.Steps = len(m.steps)
		if action.Backtrack {
			m.state.Backtracks++
		}
		switch action.Type {
		case "swipe":
			m.state.Scrolls++
			m.state.ScrollAttempts++
		case "input":
			m.state.Inputs++
		case "tap":
			if action.Recovery {
				m.state.Recoveries++
			}
		}
		last := action
		m.state.LastAction = &last
		m.refreshCountsLocked()
		m.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(config.IntervalMillis) * time.Millisecond):
		}
	}
}

func isAsyncStartupActivity(activity string) bool {
	value := strings.ToLower(activity)
	return strings.Contains(value, "splash") || strings.Contains(value, "startup") || strings.Contains(value, "launch") || strings.Contains(value, "loading")
}

func (m *Manager) persistArtifacts(config Config) {
	m.mu.Lock()
	root, identity := m.artifactRoot, m.identity
	graph := m.graphLocked()
	steps := append([]Step(nil), m.steps...)
	receipts := m.receipts.All()
	m.mu.Unlock()
	if root == "" {
		return
	}
	transaction, err := artifacts.NewDeviceSessionTransaction(root, config.Package, "Exploration")
	if err != nil {
		m.setPersistenceError(err)
		return
	}
	defer func() { _ = transaction.Abort() }()
	stagingPaths := []string{filepath.Join(transaction.Staging, "graph.json"), filepath.Join(transaction.Staging, "steps.json"), filepath.Join(transaction.Staging, "receipts.json")}
	for index, value := range []any{graph, steps, receipts} {
		content, marshalErr := json.MarshalIndent(value, "", "  ")
		if marshalErr != nil {
			err = marshalErr
			break
		}
		if writeErr := artifacts.WriteFileAtomic(stagingPaths[index], append(content, '\n'), 0o644); writeErr != nil {
			err = writeErr
			break
		}
	}
	if err == nil {
		stagingIndexPath := filepath.Join(transaction.Staging, "evidence.json")
		index := evidence.BuildForPublication(transaction.Staging, transaction.Final, config.Package, "", identity, stagingPaths)
		err = evidence.Write(stagingIndexPath, index)
		if err == nil {
			err = transaction.Commit()
		}
		if err == nil {
			err = artifacts.PruneSessions(filepath.Dir(transaction.Final), artifacts.MaxSessionsPerKind, transaction.Final)
		}
		if err == nil {
			indexPath := filepath.Join(transaction.Final, "evidence.json")
			m.mu.Lock()
			m.state.ArtifactDir, m.state.EvidenceIndexPath = filepath.ToSlash(transaction.Final), filepath.ToSlash(indexPath)
			m.mu.Unlock()
			return
		}
	}
	m.setPersistenceError(err)
}

func (m *Manager) setPersistenceError(err error) {
	if err == nil {
		return
	}
	m.mu.Lock()
	if m.state.Error == "" {
		m.state.Error = "persist exploration: " + err.Error()
	} else {
		m.state.Error += "; persist exploration: " + err.Error()
	}
	m.mu.Unlock()
}

func scopedFingerprint(fingerprint, activity string) string {
	sum := sha256.Sum256([]byte(activity + "\x00" + fingerprint))
	return fmt.Sprintf("%x", sum[:16])
}

func (m *Manager) blockEdgeLocked(state, action string) {
	key := explorationEdgeKey(state, action)
	if !m.blockedEdges[key] {
		m.blockedEdges[key] = true
		m.state.BlockedEdges++
	}
}

func inputExecutionKey(activity string, action Action) string {
	return activity + "\x00" + action.ID
}

func inputFieldKey(activity string, action Action) string {
	identity := stableInputFieldIdentity(action.ResourceID, action.Description, action.Class, action.Description, action.Bounds)
	return activity + "\x00" + identity
}

func observeExplorationCycle(guard *cycleGuard, action Action, from, actionKey, to, blockFrom string) int {
	// A bounded input corpus intentionally produces several transitions on the
	// same page. Treating those as navigation cycles creates false positives;
	// input repetition is controlled independently by inputTried.
	if action.Type == "input" {
		return 0
	}
	return guard.observe(from, actionKey, to, blockFrom)
}

func actionTimeout(action Action) time.Duration {
	if action.Type == "input" {
		return 12 * time.Second
	}
	return 5 * time.Second
}

func backAction(from, parent string) Action {
	sum := sha256.Sum256([]byte("back:" + from + ":" + parent))
	return Action{ID: fmt.Sprintf("%x", sum[:8]), Type: "back", Description: "DFS backtrack", Backtrack: true}
}

func inverseAction(action Action, from, parent string) Action {
	if action.Type != "swipe" {
		return backAction(from, parent)
	}
	sum := sha256.Sum256([]byte("reverse-swipe:" + from + ":" + parent + ":" + action.ID))
	return Action{
		ID: fmt.Sprintf("%x", sum[:8]), Type: "swipe", Direction: "backward", Backtrack: true,
		X: action.EndX, Y: action.EndY, EndX: action.X, EndY: action.Y,
		Bounds: action.Bounds, ResourceID: action.ResourceID, Class: action.Class, Description: "DFS scroll backtrack",
	}
}

func (m *Manager) executeAction(ctx context.Context, action Action) error {
	var err error
	switch action.Type {
	case "tap":
		_, err = m.executor.Run(ctx, "input", "tap", strconv.Itoa(action.X), strconv.Itoa(action.Y))
	case "swipe":
		_, err = m.executor.Run(ctx, "input", "swipe", strconv.Itoa(action.X), strconv.Itoa(action.Y), strconv.Itoa(action.EndX), strconv.Itoa(action.EndY), "350")
	case "back":
		_, err = m.executor.Run(ctx, "input", "keyevent", "BACK")
	case "input":
		if m.textInjector == nil {
			return errors.New("exploration text injector unavailable")
		}
		if _, err = m.executor.Run(ctx, "input", "tap", strconv.Itoa(action.X), strconv.Itoa(action.Y)); err != nil {
			return err
		}
		if err = waitAction(ctx, 100*time.Millisecond); err != nil {
			return err
		}
		if _, err = m.executor.Run(ctx, "input", "keycombination", "113", "29"); err != nil {
			return err
		}
		if err = waitAction(ctx, 100*time.Millisecond); err != nil {
			return err
		}
		if _, err = m.executor.Run(ctx, "input", "keyevent", "67"); err != nil {
			return err
		}
		if action.Text == "" {
			return nil
		}
		return m.textInjector.InjectText(ctx, action.Text)
	default:
		return fmt.Errorf("unsupported exploration action: %s", action.Type)
	}
	return err
}

func waitAction(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (m *Manager) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return cloneState(m.state)
}

func (m *Manager) Graph() Graph {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.graphLocked()
}

func (m *Manager) graphLocked() Graph {
	graph := Graph{States: make([]GraphState, 0, len(m.nodes)), Edges: append([]GraphEdge(nil), m.edges...)}
	for fingerprint, node := range m.nodes {
		graph.States = append(graph.States, GraphState{Fingerprint: fingerprint, Activity: node.activity, Parent: m.parents[fingerprint], NodeCount: node.analysis.NodeCount, ActionCount: len(node.analysis.Actions)})
	}
	sort.Slice(graph.States, func(i, j int) bool { return graph.States[i].Fingerprint < graph.States[j].Fingerprint })
	return graph
}

func (m *Manager) Steps() []Step {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Step(nil), m.steps...)
}

func (m *Manager) Stop(ctx context.Context) (State, error) {
	m.mu.Lock()
	if !m.state.Running && !m.state.Stopping && !m.state.Finalizing {
		state := cloneState(m.state)
		m.mu.Unlock()
		return state, nil
	}
	cancel, done := m.cancel, m.done
	if m.state.Running {
		m.stopRequested = true
		m.state.Stopping = true
	}
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	select {
	case <-done:
		return m.State(), nil
	case <-ctx.Done():
		return m.State(), ctx.Err()
	case <-time.After(5 * time.Second):
		return m.State(), errors.New("exploration stop timed out")
	}
}

func (m *Manager) StopOwned(ctx context.Context, sessionID, ownerToken string) (State, error) {
	m.mu.Lock()
	coordinator, identity := m.coordinator, m.identity
	active := m.state.Running || m.state.Stopping || m.state.Finalizing
	m.mu.Unlock()
	if sessionID != identity.SessionID || ownerToken != identity.OwnerToken {
		return m.State(), execution.ErrOwnerMismatch
	}
	if coordinator != nil && active {
		if err := coordinator.Validate(sessionID, ownerToken); err != nil {
			return m.State(), err
		}
	}
	return m.Stop(ctx)
}

func (m *Manager) finish(reason, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.finishLocked(reason, message)
}

func (m *Manager) finishLocked(reason, message string) {
	if !m.state.Running {
		return
	}
	if m.stopRequested {
		reason, message = "stopped", ""
	}
	now := time.Now().UTC()
	m.state.Running = false
	m.state.Stopping = false
	m.state.Finalizing = true
	m.state.EndedAt = &now
	m.state.StopReason = reason
	m.state.Error = message
	m.refreshCountsLocked()
}

func (m *Manager) refreshCountsLocked() {
	m.state.DiscoveredStates = len(m.nodes)
	m.state.DiscoveredEdges = len(m.edges)
	m.state.ObservedActivities = len(m.activities)
	m.state.ActivityCovered = len(m.activities)
	m.state.InputFields = len(m.inputFields)
	m.state.ActivityTotal = len(m.config.ExpectedActivities)
	m.state.ActivityCoverage = nil
	if m.state.ActivityTotal > 0 {
		covered := 0
		for _, activity := range m.config.ExpectedActivities {
			if _, exists := m.activities[activity]; exists {
				covered++
			}
		}
		m.state.ActivityCovered = covered
		percent := float64(covered) * 100 / float64(m.state.ActivityTotal)
		m.state.ActivityCoverage = &percent
	}
}

func cloneState(state State) State {
	if state.LastCycle != nil {
		last := *state.LastCycle
		state.LastCycle = &last
	}
	if state.LastAction != nil {
		last := *state.LastAction
		state.LastAction = &last
	}
	if state.LastSpecial != nil {
		special := *state.LastSpecial
		if special.Action != nil {
			action := *special.Action
			special.Action = &action
		}
		state.LastSpecial = &special
	}
	if state.ActivityCoverage != nil {
		coverage := *state.ActivityCoverage
		state.ActivityCoverage = &coverage
	}
	if state.LastSpecial != nil {
		last := *state.LastSpecial
		if last.Action != nil {
			action := *last.Action
			last.Action = &action
		}
		state.LastSpecial = &last
	}
	return state
}
