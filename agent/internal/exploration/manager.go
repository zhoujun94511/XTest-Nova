package exploration

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/companioncontrol"
	"github.com/zhoujun94511/xtest-nova/agent/internal/execution"
	"github.com/zhoujun94511/xtest-nova/agent/internal/platform"
)

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

type FocusSource interface {
	ForegroundFocus(context.Context) (component, windowVersion string, err error)
}

type TextInjector interface {
	InjectText(context.Context, string) error
}

type loopSignal int

const (
	loopNext loopSignal = iota
	loopContinue
	loopRestart
	loopStop
)

type runLoop struct {
	pending                 int
	pendingNoteBlocked      bool
	escalationArmed         string
	escalationCoolState     string
	escalationCooldownUntil int
	specialAttempts         map[string]int
	specialCooldownUntil    map[string]time.Time
	specialWaitStarted      map[string]time.Time
	externalAttempts        map[string]int
	settleUntil             time.Time
	settleGeneration        pageGeneration
	lastOnTarget            pageGeneration
	settleForeground        string
	emptyCaptures           int
	exhaustedFingerprint    string
	exhaustedCaptures       int
	actionAttemptNumber     int
	initialSettleUntil      time.Time
	startupGeneration       pageGeneration
	startupReleased         bool
	adActive                bool
	adEncounterSequence     int
	adStartedAt             time.Time
	nextBattery             time.Time
	renderActivity          string
	renderAttempts          int
	screenWidth             int
	screenHeight            int
	restartedActivity       string
	escapeActivity          string
	escapeAttempts          int
}

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
	activityMemory       *activityMemory
	persistArtifactsHook func(Config)
	targetReplay         func(context.Context, string, string, string) error
	playedTargets        map[string]bool
	targetReplayAfter    map[string]time.Time
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
	foreground, activity, _, hierarchy, err := m.capture(ctx)
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

func (m *Manager) capture(ctx context.Context) (foreground, activity, windowVersion, hierarchy string, err error) {
	foreground, activity, windowVersion, err = m.currentForeground(ctx)
	if err != nil {
		return "", "", "", "", err
	}
	hierarchy, err = m.hierarchy.Hierarchy(ctx)
	if err != nil {
		return "", "", "", "", err
	}
	afterForeground, afterActivity, afterVersion, err := m.currentForeground(ctx)
	if err != nil {
		return "", "", "", "", err
	}
	if foreground != afterForeground || activity != afterActivity {
		return "", "", "", "", fmt.Errorf("%w: %s -> %s", errUnstableCapture, activity, afterActivity)
	}
	if afterVersion != "" {
		windowVersion = afterVersion
	}
	return foreground, activity, windowVersion, hierarchy, nil
}

func (m *Manager) currentForeground(ctx context.Context) (foreground, activity, windowVersion string, err error) {
	if source, ok := m.foreground.(FocusSource); ok {
		activity, windowVersion, err = source.ForegroundFocus(ctx)
		if separator := strings.IndexByte(activity, '/'); separator >= 0 {
			foreground = activity[:separator]
		}
	} else if source, ok := m.foreground.(ActivitySource); ok {
		activity, err = source.ForegroundActivity(ctx)
		if separator := strings.IndexByte(activity, '/'); separator >= 0 {
			foreground = activity[:separator]
		}
	} else {
		foreground, err = m.foreground.ForegroundPackage(ctx)
	}
	if err != nil {
		return "", "", "", err
	}
	return foreground, activity, windowVersion, nil
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
	m.playedTargets = map[string]bool{}
	m.targetReplayAfter = map[string]time.Time{}
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
		MaxSteps: config.MaxSteps, DurationSeconds: config.DurationSeconds,
		EnableScroll: config.EnableScroll, EnableBacktrack: config.EnableBacktrack,
		RecoverPopups: config.RecoverPopups, MaxBacktracks: config.MaxBacktracks,
	}
	state := m.state
	m.mu.Unlock()
	go m.run(ctx, config, m.done)
	return state, nil
}

func (m *Manager) run(ctx context.Context, config Config, done chan struct{}) {
	m.mu.Lock()
	memory := loadActivityMemory(m.artifactRoot, config.Package, config.VersionCode)
	m.activityMemory = memory
	m.mu.Unlock()
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
		if memory != nil && m.done == done {
			_ = memory.save()
		}
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
	var restoreOverlay bool
	overlaySupported := canControlCompanionOverlay(m.executor)
	if overlaySupported {
		overlayContext, cancelOverlay := context.WithTimeout(ctx, 3*time.Second)
		restoreOverlay, overlayErr = companioncontrol.SuppressOverlay(overlayContext, m.executor)
		cancelOverlay()
	}
	m.mu.Lock()
	if restoreOverlay && overlayErr == nil {
		m.state.OverlaySuppressed = true
	} else if overlayErr != nil {
		m.state.OverlayControlError = overlayErr.Error()
	}
	m.mu.Unlock()
	if restoreOverlay && overlayErr == nil {
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
	loop := &runLoop{
		pending: -1, specialAttempts: map[string]int{}, specialCooldownUntil: map[string]time.Time{},
		specialWaitStarted: map[string]time.Time{}, externalAttempts: map[string]int{},
		initialSettleUntil: time.Now().Add(initialAsyncSettleDuration),
	}
	defer func() {
		if loop.adActive && !loop.adStartedAt.IsZero() {
			m.mu.Lock()
			m.state.AdWaitMillis += time.Since(loop.adStartedAt).Milliseconds()
			m.mu.Unlock()
		}
	}()
	consecutiveUnstableCaptures := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		foreground, activity, windowVersion, hierarchy, err := m.capture(ctx)
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
				if err := waitAction(ctx, time.Duration(config.IntervalMillis)*time.Millisecond); err != nil {
					return
				}
				continue
			}
			m.finish("safety_stop", err.Error())
			return
		}
		consecutiveUnstableCaptures = 0
		if loop.restartedActivity != "" && time.Now().Before(loop.settleUntil) {
			if err := waitAction(ctx, time.Duration(config.IntervalMillis)*time.Millisecond); err != nil {
				return
			}
			continue
		}
		switch m.handleSpecialScene(ctx, config, loop, foreground, activity, windowVersion, hierarchy) {
		case loopStop:
			return
		case loopContinue:
			continue
		case loopRestart:
			if err := m.runRestart(ctx, config, loop, activity, pageGeneration{activity: activity, windowVersion: windowVersion}); err != nil {
				m.finish("safety_stop", fmt.Sprintf("restart target after the special scene: %v", err))
				return
			}
			if err := waitAction(ctx, time.Duration(config.IntervalMillis)*time.Millisecond); err != nil {
				return
			}
			continue
		}
		switch m.recoverOffTarget(ctx, config, loop, foreground, activity, windowVersion) {
		case loopStop:
			return
		case loopContinue:
			continue
		}
		// Special-scene actions and controlled relaunches often land before the
		// next page is ready. Keep inspecting special scenes above, and build
		// the ordinary graph once the activity or window relayout sequence changes.
		// The previous deadline remains the upper bound when neither changes.
		currentGeneration := pageGeneration{activity: activity, windowVersion: windowVersion}
		previousOnTarget := loop.lastOnTarget
		if foreground == config.Package && !settleReady(time.Now(), loop.settleUntil, loop.settleGeneration, previousOnTarget, currentGeneration) {
			loop.lastOnTarget = currentGeneration
			if err := waitAction(ctx, time.Duration(config.IntervalMillis)*time.Millisecond); err != nil {
				return
			}
			continue
		}
		if foreground == config.Package {
			loop.lastOnTarget = currentGeneration
		}
		analysis, err := AnalyzeWithInputStrategy(hierarchy, config.Package, config.Rules, config.Seed, config.InputStrategy)
		if err != nil {
			m.finish("safety_stop", err.Error())
			return
		}
		analysis.Activity = activity
		analysis.Fingerprint = scopedFingerprint(analysis.Fingerprint, activity)
		releasedBefore := loop.startupReleased
		holdStartup := observeStartup(time.Now(), loop.initialSettleUntil, activity, &loop.startupGeneration, &loop.startupReleased, currentGeneration)
		if !releasedBefore && loop.startupReleased {
			loop.emptyCaptures = 0
		}
		if len(analysis.Actions) == 0 {
			loop.emptyCaptures++
		}
		if len(analysis.Actions) == 0 && deferEmptyPage(holdStartup, time.Now(), loop.settleUntil, loop.settleGeneration, previousOnTarget, currentGeneration, loop.emptyCaptures) {
			m.mu.Lock()
			elapsed := m.durationElapsedLocked(config)
			if elapsed {
				m.finishLocked("duration_elapsed", "")
			}
			m.mu.Unlock()
			if elapsed {
				return
			}
			if m.stopForLowBattery(ctx, config, loop) == loopStop {
				return
			}
			if err := waitAction(ctx, time.Duration(config.IntervalMillis)*time.Millisecond); err != nil {
				return
			}
			continue
		}
		if len(analysis.Actions) > 0 {
			loop.emptyCaptures = 0
		}
		switch m.applySessionGuards(ctx, config, loop, activity, hierarchy, &analysis) {
		case loopStop:
			return
		case loopContinue:
			if err := waitAction(ctx, time.Duration(config.IntervalMillis)*time.Millisecond); err != nil {
				return
			}
			continue
		}
		m.mu.Lock()
		node := m.recordPageLocked(config, &analysis, activity, loop)
		if m.durationElapsedLocked(config) {
			m.finishLocked("duration_elapsed", "")
			m.mu.Unlock()
			return
		}
		if stepBudgetReached(config, m.state.Steps) {
			m.finishLocked("max_steps", "")
			m.mu.Unlock()
			return
		}
		choice := m.chooseActionLocked(config, node, analysis, activity, loop)
		if choice.signal != loopNext {
			m.mu.Unlock()
			if choice.signal == loopStop {
				return
			}
			if choice.signal == loopRestart {
				if err := m.runRestart(ctx, config, loop, activity, currentGeneration); err != nil {
					m.finish("safety_stop", fmt.Sprintf("restart target after the graph round: %v", err))
					return
				}
			}
			if err := waitAction(ctx, time.Duration(config.IntervalMillis)*time.Millisecond); err != nil {
				return
			}
			continue
		}
		if m.executeChosen(ctx, config, loop, node, analysis, activity, choice.action) == loopStop {
			return
		}
	}
}

func (m *Manager) ExecutionIdentity() (execution.Identity, bool) {
	if m == nil {
		return execution.Identity{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.identity.SessionID == "" || (!m.state.Running && !m.state.Stopping && !m.state.Finalizing) {
		return execution.Identity{}, false
	}
	return m.identity, true
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
	case <-time.After(120 * time.Second):
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
