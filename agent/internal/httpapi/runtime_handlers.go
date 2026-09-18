package httpapi

import (
	"net/http"
	"runtime"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/runner"
)

type sessionSnapshot struct {
	Runner       bool `json:"runner"`
	Exploration  bool `json:"exploration"`
	Recording    bool `json:"recording"`
	Replay       bool `json:"replay"`
	ScreenRecord bool `json:"screenRecord"`
	Performance  bool `json:"performance"`
}

type runtimeSnapshot struct {
	Version      string          `json:"version"`
	StartedAt    time.Time       `json:"startedAt"`
	UptimeMillis int64           `json:"uptimeMillis"`
	Goroutines   int             `json:"goroutines"`
	HeapAlloc    uint64          `json:"heapAllocBytes"`
	HeapObjects  uint64          `json:"heapObjects"`
	SystemBytes  uint64          `json:"systemBytes"`
	GCCycles     uint32          `json:"gcCycles"`
	Sessions     sessionSnapshot `json:"sessions"`
	Hierarchy    any             `json:"hierarchy,omitempty"`
}

type hierarchyDiagnosticsProvider interface {
	Diagnostics() any
}

type sessionActivity struct {
	Running    bool   `json:"running"`
	Stopping   bool   `json:"stopping,omitempty"`
	Finalizing bool   `json:"finalizing,omitempty"`
	Error      string `json:"error,omitempty"`
}

type sessionActivities struct {
	Runner       sessionActivity `json:"runner"`
	Exploration  sessionActivity `json:"exploration"`
	Recording    sessionActivity `json:"recording"`
	Replay       sessionActivity `json:"replay"`
	ScreenRecord sessionActivity `json:"screenRecord"`
	Performance  sessionActivity `json:"performance"`
}

type sessionActivityResponse struct {
	Active   bool              `json:"active"`
	Sessions sessionActivities `json:"sessions"`
}

type runnerActivityProvider interface {
	Activity() runner.ActivityState
}

func (a *API) registerRuntimeRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /v1/diagnostics/runtime", a.runtimeDiagnostics)
	m.HandleFunc("GET /v1/sessions/current", a.sessionActivities)
}

func (a *API) sessionActivitySnapshot() sessionActivities {
	result := sessionActivities{}
	if a.runner != nil {
		if provider, ok := a.runner.(runnerActivityProvider); ok {
			state := provider.Activity()
			result.Runner = sessionActivity{Running: state.Running, Finalizing: state.Finalizing, Error: state.Error}
		} else {
			state := a.runner.State()
			result.Runner = sessionActivity{Running: state.Running, Finalizing: state.Finalizing, Error: state.Error}
		}
	}
	if a.explorer != nil {
		state := a.explorer.State()
		result.Exploration = sessionActivity{Running: state.Running, Stopping: state.Stopping, Finalizing: state.Finalizing, Error: state.Error}
	}
	if a.recordReplay != nil {
		recording := a.recordReplay.RecordingState()
		replay := a.recordReplay.ReplayState()
		result.Recording = sessionActivity{Running: recording.Running, Stopping: recording.Stopping, Finalizing: recording.Finalizing, Error: recording.Error}
		result.Replay = sessionActivity{Running: replay.Running, Stopping: replay.Stopping, Finalizing: replay.Finalizing, Error: replay.Error}
	}
	if a.recorder != nil {
		result.ScreenRecord.Running = a.recorder.Running()
	}
	if a.performance != nil {
		state := a.performance.State()
		result.Performance = sessionActivity{Running: state.Running, Error: state.Error}
	}
	return result
}

func (s sessionActivities) active() bool {
	return activityBusy(s.Runner) || activityBusy(s.Exploration) || activityBusy(s.Recording) ||
		activityBusy(s.Replay) || activityBusy(s.ScreenRecord) || activityBusy(s.Performance)
}

func activityBusy(state sessionActivity) bool {
	return state.Running || state.Stopping || state.Finalizing
}

func (s sessionActivities) legacySnapshot() sessionSnapshot {
	return sessionSnapshot{
		Runner: activityBusy(s.Runner), Exploration: activityBusy(s.Exploration),
		Recording: activityBusy(s.Recording), Replay: activityBusy(s.Replay), ScreenRecord: activityBusy(s.ScreenRecord),
		Performance: activityBusy(s.Performance),
	}
}

func (a *API) sessionActivities(w http.ResponseWriter, _ *http.Request) {
	sessions := a.sessionActivitySnapshot()
	writeJSON(w, http.StatusOK, sessionActivityResponse{Active: sessions.active(), Sessions: sessions})
}

func (a *API) runtimeDiagnostics(w http.ResponseWriter, _ *http.Request) {
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	sessions := a.sessionActivitySnapshot().legacySnapshot()
	snapshot := runtimeSnapshot{
		Version: Version, StartedAt: a.startedAt, UptimeMillis: time.Since(a.startedAt).Milliseconds(),
		Goroutines: runtime.NumGoroutine(), HeapAlloc: memory.HeapAlloc, HeapObjects: memory.HeapObjects,
		SystemBytes: memory.Sys, GCCycles: memory.NumGC, Sessions: sessions,
	}
	if diagnostics, ok := a.automation.(hierarchyDiagnosticsProvider); ok {
		snapshot.Hierarchy = diagnostics.Diagnostics()
	}
	writeJSON(w, http.StatusOK, snapshot)
}
