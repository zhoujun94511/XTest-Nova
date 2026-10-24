package recordreplay

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/artifacts"
	"github.com/zhoujun94511/xtest-nova/agent/internal/evidence"
	"github.com/zhoujun94511/xtest-nova/agent/internal/execution"
)

func (m *Manager) writeDraftLocked() error {
	if m.draftPath == "" {
		return nil
	}
	return writeJSONAtomic(m.draftPath, m.current)
}

func (m *Manager) finalizeRecording(snapshot recordingSnapshot, hook func(recordingSnapshot) (recordingResult, error), coordinator *execution.Coordinator, done chan struct{}) {
	var result recordingResult
	var err error
	if hook != nil {
		result, err = hook(snapshot)
	} else {
		result, err = persistRecording(snapshot)
	}
	if err != nil {
		m.mu.Lock()
		if m.recordIdentity.SessionID == snapshot.identity.SessionID {
			m.recording.Error = err.Error()
			m.recording.Finalizing = false
			m.recordCancel, m.recordDone = nil, nil
			m.recordFinalizeErr = err
			m.recordPersisting = false
		}
		m.mu.Unlock()
		if coordinator != nil {
			coordinator.Release(snapshot.identity)
		}
		close(done)
		return
	}
	if snapshot.draftPath != "" {
		_ = os.RemoveAll(filepath.Dir(snapshot.draftPath))
	}
	now := time.Now().UTC()
	m.mu.Lock()
	if m.recordIdentity.SessionID == snapshot.identity.SessionID {
		m.current = result.value
		m.recording.Running, m.recording.Stopping = false, false
		m.recording.EndedAt, m.recording.Path = &now, result.path
		m.recording.CaseFingerprint = result.value.Integrity
		m.recording.EvidenceIndexPath = filepath.ToSlash(result.evidencePath)
		m.recording.DraftPath = ""
		m.recordCancel, m.recordDone = nil, nil
		m.draftPath = ""
		m.recordingAssets = nil
	}
	m.mu.Unlock()
	if coordinator != nil {
		coordinator.Release(snapshot.identity)
	}
	m.mu.Lock()
	if m.recordIdentity.SessionID == snapshot.identity.SessionID {
		m.recording.Finalizing = false
		m.recordFinalizeErr = nil
		m.recordPersisting = false
	}
	m.mu.Unlock()
	close(done)
}

func cloneRecordingAssets(source map[string][]byte) map[string][]byte {
	result := make(map[string][]byte, len(source))
	for path, data := range source {
		result[path] = append([]byte(nil), data...)
	}
	return result
}

func persistRecording(snapshot recordingSnapshot) (recordingResult, error) {
	value := snapshot.value
	if err := value.Seal(); err != nil {
		return recordingResult{}, err
	}
	directory := filepath.Join(snapshot.root, value.Package, "Replay")
	if value.Task != "" {
		directory = filepath.Join(directory, value.Task)
	}
	directory = filepath.Join(directory, value.RecordedAt.Format("20060102T150405.000000000Z"))
	transaction, err := artifacts.BeginDirectory(directory)
	if err != nil {
		return recordingResult{}, err
	}
	defer func() { _ = transaction.Abort() }()
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return recordingResult{}, err
	}
	content = append(content, '\n')
	path := filepath.Join(transaction.Staging, "case.json")
	if err = writeBytesAtomic(path, content); err != nil {
		return recordingResult{}, err
	}
	evidenceFiles := []string{path}
	manifestFiles := []RecordingManifestFile{{Path: "case.json", SHA256: sha256Hex(content), SizeBytes: int64(len(content))}}
	for referencePath, data := range snapshot.assets {
		assetPath := filepath.Join(transaction.Staging, filepath.FromSlash(referencePath))
		if err = writeBytesAtomic(assetPath, data); err != nil {
			return recordingResult{}, err
		}
		evidenceFiles = append(evidenceFiles, assetPath)
		manifestFiles = append(manifestFiles, RecordingManifestFile{Path: referencePath, SHA256: sha256Hex(data), SizeBytes: int64(len(data))})
	}
	sort.Slice(manifestFiles, func(i, j int) bool { return manifestFiles[i].Path < manifestFiles[j].Path })
	manifest := RecordingManifest{SchemaVersion: "xtest-nova-recording-manifest/v1", Package: value.Package, Name: value.Name, Task: value.Task, RecordedAt: value.RecordedAt, RecordedWidth: value.RecordedWidth, RecordedHeight: value.RecordedHeight, Actions: len(value.Actions), CaseFingerprint: value.Integrity, Files: manifestFiles}
	manifestPath := filepath.Join(transaction.Staging, "manifest.json")
	if err = writeJSONAtomic(manifestPath, manifest); err != nil {
		return recordingResult{}, err
	}
	evidenceFiles = append(evidenceFiles, manifestPath)
	evidencePath := filepath.Join(transaction.Staging, "evidence.json")
	index := evidence.BuildForPublication(transaction.Staging, transaction.Final, value.Package, value.Integrity, snapshot.identity, evidenceFiles)
	if err = evidence.Write(evidencePath, index); err != nil {
		return recordingResult{}, err
	}
	if err = transaction.Commit(); err != nil {
		return recordingResult{}, err
	}
	finalPath := filepath.Join(transaction.Final, "case.json")
	finalEvidencePath := filepath.Join(transaction.Final, "evidence.json")
	if err = pruneCases(filepath.Join(snapshot.root, value.Package, "Replay"), filepath.Dir(finalPath), 200); err != nil {
		return recordingResult{}, err
	}
	return recordingResult{value: value, path: filepath.ToSlash(finalPath), evidencePath: finalEvidencePath}, nil
}

func writeJSONAtomic(path string, value any) error {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeBytesAtomic(path, append(content, '\n'))
}

func writeBytesAtomic(path string, content []byte) error {
	return artifacts.WriteFileAtomic(path, content, 0o644)
}

func sha256Hex(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

func (m *Manager) Cases(packageName string) ([]CaseSummary, error) {
	packageName = strings.TrimSpace(packageName)
	if packageName != "" && !packagePattern.MatchString(packageName) {
		return nil, errors.New("invalid Android package name")
	}
	result := make([]CaseSummary, 0)
	searchRoots := make([]string, 0)
	if packageName != "" {
		searchRoots = append(searchRoots, filepath.Join(m.root, packageName, "Replay"))
	} else if packages, readErr := os.ReadDir(m.root); readErr == nil {
		for _, entry := range packages {
			if entry.IsDir() && packagePattern.MatchString(entry.Name()) {
				searchRoots = append(searchRoots, filepath.Join(m.root, entry.Name(), "Replay"))
			}
		}
	} else if !os.IsNotExist(readErr) {
		return nil, readErr
	}
	for _, searchRoot := range searchRoots {
		err := filepath.Walk(searchRoot, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				if os.IsNotExist(walkErr) {
					return nil
				}
				return walkErr
			}
			if info.IsDir() || info.Name() != "case.json" || info.Size() > 4<<20 {
				return nil
			}
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}
			var value Case
			if json.Unmarshal(content, &value) != nil || value.Verify() != nil || (packageName != "" && value.Package != packageName) {
				return nil
			}
			relative, relativeErr := filepath.Rel(m.root, path)
			if relativeErr != nil {
				return nil
			}
			result = append(result, CaseSummary{
				ID:   base64.RawURLEncoding.EncodeToString([]byte(filepath.ToSlash(relative))),
				Name: value.Name, Package: value.Package, RecordedAt: value.RecordedAt, Actions: len(value.Actions), Task: value.Task, CaseFingerprint: value.Integrity,
			})
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].RecordedAt.After(result[j].RecordedAt) })
	if len(result) > 200 {
		result = result[:200]
	}
	return result, nil
}

func (m *Manager) Drafts(packageName string) ([]DraftSummary, error) {
	packageName = strings.TrimSpace(packageName)
	if !packagePattern.MatchString(packageName) {
		return nil, errors.New("invalid Android package name")
	}
	root := filepath.Join(m.root, packageName, "Replay", ".drafts")
	result := make([]DraftSummary, 0)
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return walkErr
		}
		if info.IsDir() || info.Name() != "draft.json" || info.Size() > 4<<20 {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		var value Case
		if json.Unmarshal(content, &value) != nil || value.Validate() != nil || value.Package != packageName {
			return nil
		}
		relative, relativeErr := filepath.Rel(m.root, path)
		if relativeErr != nil {
			return nil
		}
		result = append(result, DraftSummary{ID: base64.RawURLEncoding.EncodeToString([]byte(filepath.ToSlash(relative))), Name: value.Name, Package: value.Package, RecordedAt: value.RecordedAt, Actions: len(value.Actions), Task: value.Task})
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	sort.Slice(result, func(i, j int) bool { return result[i].RecordedAt.After(result[j].RecordedAt) })
	return result, nil
}

func (m *Manager) draftPathFromID(id string) (string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil || len(decoded) == 0 {
		return "", errors.New("invalid recording draft id")
	}
	relative := filepath.Clean(filepath.FromSlash(string(decoded)))
	if filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.Base(relative) != "draft.json" {
		return "", errors.New("invalid recording draft path")
	}
	segments := strings.Split(filepath.ToSlash(relative), "/")
	validDraftPath := false
	for index := 0; index+1 < len(segments); index++ {
		if segments[index] == ".drafts" && segments[index+1] != "" {
			validDraftPath = true
			break
		}
	}
	if !validDraftPath {
		return "", errors.New("invalid recording draft path")
	}
	return filepath.Join(m.root, relative), nil
}

func (m *Manager) FinalizeDraft(ctx context.Context, id string) (RecordingState, error) {
	path, err := m.draftPathFromID(id)
	if err != nil {
		return m.RecordingState(), err
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() > 4<<20 {
		if err == nil {
			err = errors.New("invalid recording draft file")
		}
		return m.RecordingState(), err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return m.RecordingState(), err
	}
	var value Case
	if err = json.Unmarshal(content, &value); err != nil {
		return m.RecordingState(), err
	}
	value.Integrity = ""
	if err = value.Validate(); err != nil {
		return m.RecordingState(), err
	}
	assets := map[string][]byte{}
	for _, action := range value.Actions {
		if action.ReferencePath == "" {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(filepath.Dir(path), filepath.FromSlash(action.ReferencePath)))
		if readErr != nil {
			return m.RecordingState(), fmt.Errorf("recording draft reference %s: %w", action.ReferencePath, readErr)
		}
		assets[action.ReferencePath] = data
	}
	m.mu.Lock()
	if m.recording.Running || m.recording.Stopping || m.recording.Finalizing ||
		m.replay.Running || m.replay.Stopping || m.replay.Finalizing || m.recordingPendingLocked() {
		m.mu.Unlock()
		return m.RecordingState(), errors.New("record or replay session already active")
	}
	now := time.Now().UTC()
	requestID := "recover-" + strconv.FormatInt(now.UnixNano(), 10)
	m.current = value
	m.recordGeneration++
	m.recordIdentity = execution.NewIdentity("recording-recovery", requestID, m.recordGeneration)
	m.recording = RecordingState{RequestID: requestID, Identity: m.recordIdentity, Package: value.Package, Task: value.Task, Name: value.Name, StartedAt: &value.RecordedAt, EndedAt: &now, Actions: len(value.Actions), DraftPath: filepath.ToSlash(path)}
	m.draftPath = path
	m.recordingAssets = assets
	m.mu.Unlock()
	return m.StopRecording(ctx)
}

func (m *Manager) DeleteDraft(id string) error {
	path, err := m.draftPathFromID(id)
	if err != nil {
		return err
	}
	m.mu.Lock()
	activeDraft := m.draftPath != "" && filepath.Clean(m.draftPath) == filepath.Clean(path) && m.current.SchemaVersion != "" && m.recording.Path == ""
	m.mu.Unlock()
	if activeDraft {
		return errors.New("active recording draft cannot be deleted")
	}
	return os.RemoveAll(filepath.Dir(path))
}

func pruneCases(root, keep string, limit int) error {
	type storedCase struct {
		directory string
		modified  time.Time
	}
	values := make([]storedCase, 0)
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !info.IsDir() && info.Name() == "case.json" {
			values = append(values, storedCase{directory: filepath.Dir(path), modified: info.ModTime()})
		}
		return nil
	})
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	sort.Slice(values, func(i, j int) bool { return values[i].modified.Before(values[j].modified) })
	for len(values) > limit {
		value := values[0]
		values = values[1:]
		if filepath.Clean(value.directory) == filepath.Clean(keep) {
			values = append(values, value)
			continue
		}
		if err = os.RemoveAll(value.directory); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) caseJSONPath(id string) (string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil || len(decoded) == 0 {
		return "", errors.New("invalid recording id")
	}
	relative := filepath.Clean(filepath.FromSlash(string(decoded)))
	if filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.Base(relative) != "case.json" {
		return "", errors.New("invalid recording path")
	}
	path := filepath.Join(m.root, relative)
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() || info.Size() > 4<<20 {
		return "", errors.New("invalid recording file")
	}
	return path, nil
}

func (m *Manager) DeleteCase(id string) error {
	path, err := m.caseJSONPath(id)
	if err != nil {
		return err
	}
	directory := filepath.Dir(path)
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var value Case
	if err = json.Unmarshal(content, &value); err != nil {
		return err
	}
	m.mu.Lock()
	recordingBusy := m.recording.Running || m.recording.Stopping || m.recording.Finalizing || m.recordingPendingLocked()
	recordingDir := ""
	if m.recording.Path != "" {
		recordingDir = filepath.Clean(filepath.Dir(filepath.FromSlash(m.recording.Path)))
	}
	replayBusy := m.replay.Running || m.replay.Stopping || m.replay.Finalizing
	replayFingerprint := m.replay.CaseFingerprint
	m.mu.Unlock()
	if recordingBusy && recordingDir != "" && recordingDir == filepath.Clean(directory) {
		return errors.New("active recording case cannot be deleted")
	}
	if replayBusy && replayFingerprint != "" && replayFingerprint == value.Integrity {
		return errors.New("active replay case cannot be deleted")
	}
	return os.RemoveAll(directory)
}

func (m *Manager) LoadCase(id string) (Case, error) {
	path, err := m.caseJSONPath(id)
	if err != nil {
		return Case{}, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return Case{}, err
	}
	var value Case
	if err = json.Unmarshal(content, &value); err != nil {
		return Case{}, err
	}
	if err = value.Verify(); err != nil {
		return Case{}, err
	}
	return value, nil
}

func (m *Manager) persistReplay(identity execution.Identity) error {
	m.mu.Lock()
	state := m.replay
	hasAssertions := m.replayHasAssertions
	var receipts []execution.ActionReceipt
	if m.replayReceipts != nil {
		receipts = m.replayReceipts.All()
	}
	m.mu.Unlock()
	// The owner token is a live-session capability. Persist the remaining
	// correlation identity, but never copy that capability into an artifact.
	state.Running = false
	state.Stopping = false
	state.Finalizing = false
	state.Identity.OwnerToken = ""
	transaction, err := artifacts.NewDeviceSessionTransaction(m.root, state.Package, "ReplayRuns")
	if err != nil {
		return fmt.Errorf("create replay artifact directory: %w", err)
	}
	defer func() { _ = transaction.Abort() }()
	state.ArtifactDir = filepath.ToSlash(transaction.Final)
	state.EvidenceIndexPath = filepath.ToSlash(filepath.Join(transaction.Final, "evidence.json"))
	if state.StopReason == "assertion_failed" {
		state.Verdict, state.VerdictReason = "failed", "screenshot checkpoint failed"
	} else if state.StopReason != "completed" || state.Error != "" {
		state.Verdict, state.VerdictReason = "not_tested", "replay did not complete reliably"
	} else if hasAssertions && len(receipts) == state.Actions {
		state.Verdict, state.VerdictReason = "passed", "all screenshot checkpoints passed with action receipts"
	} else {
		state.Verdict, state.VerdictReason = "not_tested", "replay completed without acceptance checkpoints"
	}
	paths := []string{filepath.Join(transaction.Staging, "replay.json"), filepath.Join(transaction.Staging, "receipts.json")}
	for index, value := range []any{state, receipts} {
		if err := writeJSONAtomic(paths[index], value); err != nil {
			return fmt.Errorf("write replay artifact %s: %w", filepath.Base(paths[index]), err)
		}
	}
	index := evidence.BuildForPublication(transaction.Staging, transaction.Final, state.Package, state.CaseFingerprint, identity, paths)
	if err := evidence.Write(filepath.Join(transaction.Staging, "evidence.json"), index); err != nil {
		return fmt.Errorf("write replay evidence index: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("publish replay artifacts: %w", err)
	}
	if err := artifacts.PruneSessions(filepath.Dir(transaction.Final), artifacts.MaxSessionsPerKind, transaction.Final); err != nil {
		return fmt.Errorf("prune replay artifacts: %w", err)
	}
	m.mu.Lock()
	m.replay.ArtifactDir, m.replay.EvidenceIndexPath, m.replay.Verdict, m.replay.VerdictReason = state.ArtifactDir, state.EvidenceIndexPath, state.Verdict, state.VerdictReason
	m.mu.Unlock()
	return nil
}
