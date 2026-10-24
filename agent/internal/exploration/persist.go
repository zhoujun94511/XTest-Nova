package exploration

import (
	"encoding/json"
	"path/filepath"

	"github.com/zhoujun94511/xtest-nova/agent/internal/artifacts"
	"github.com/zhoujun94511/xtest-nova/agent/internal/evidence"
)

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
