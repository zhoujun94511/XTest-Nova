package exploration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/zhoujun94511/xtest-nova/agent/internal/artifacts"
)

const activityMemorySchema = "xtest-nova-activity-memory/v1"
const maxRememberedActions = 4096
const maxRememberedTargets = 48

// activityMemory records which activities an action has reached. It is keyed
// by package and version so an upgrade does not reuse another build's counts.
type activityMemory struct {
	path        string
	packageName string
	versionCode int64
	corrupt     bool
	actions     map[string]map[string]int
}

type activityMemoryFile struct {
	SchemaVersion string                 `json:"schemaVersion"`
	Package       string                 `json:"package"`
	VersionCode   int64                  `json:"versionCode"`
	Actions       []activityMemoryAction `json:"actions"`
}

type activityMemoryAction struct {
	Key     string                 `json:"key"`
	Targets []activityMemoryTarget `json:"targets"`
}

type activityMemoryTarget struct {
	Activity string `json:"activity"`
	Count    int    `json:"count"`
}

func activityMemoryPath(root, packageName string, versionCode int64) string {
	if root == "" || !packagePattern.MatchString(packageName) || versionCode <= 0 {
		return ""
	}
	return filepath.Join(root, packageName, "ActivityMemory", strconv.FormatInt(versionCode, 10)+".json")
}

func loadActivityMemory(root, packageName string, versionCode int64) *activityMemory {
	memory := &activityMemory{
		path:        activityMemoryPath(root, packageName, versionCode),
		packageName: packageName,
		versionCode: versionCode,
		actions:     map[string]map[string]int{},
	}
	if memory.path == "" {
		return memory
	}
	content, err := os.ReadFile(memory.path)
	if err != nil {
		if !os.IsNotExist(err) {
			memory.corrupt = true
			memory.path = ""
		}
		return memory
	}
	var file activityMemoryFile
	if err := json.Unmarshal(content, &file); err != nil || file.SchemaVersion != activityMemorySchema || file.Package != packageName || file.VersionCode != versionCode {
		memory.corrupt = true
		memory.path = ""
		return memory
	}
	for _, action := range file.Actions {
		if action.Key == "" {
			continue
		}
		targets := map[string]int{}
		for _, target := range action.Targets {
			if target.Activity == "" || target.Count <= 0 {
				continue
			}
			targets[target.Activity] += target.Count
		}
		if len(targets) > 0 {
			memory.actions[action.Key] = targets
		}
	}
	return memory
}

func (m *activityMemory) note(key, activity string) {
	if m == nil || key == "" || activity == "" || key == "backtrack" {
		return
	}
	targets := m.actions[key]
	if targets == nil {
		if len(m.actions) >= maxRememberedActions {
			return
		}
		targets = map[string]int{}
		m.actions[key] = targets
	}
	if _, ok := targets[activity]; !ok && len(targets) >= maxRememberedTargets {
		return
	}
	targets[activity]++
}

// unseenFraction reports whether key was seen before and what share of its
// historical destinations are absent from visited.
func (m *activityMemory) unseenFraction(key string, visited map[string]struct{}) (bool, float64) {
	if m == nil || key == "" {
		return false, 0
	}
	targets := m.actions[key]
	if len(targets) == 0 {
		return false, 0
	}
	total := 0
	unseen := 0
	for activity, count := range targets {
		if count <= 0 {
			continue
		}
		total += count
		if _, ok := visited[activity]; !ok {
			unseen += count
		}
	}
	if total == 0 {
		return false, 0
	}
	return true, float64(unseen) / float64(total)
}

func (m *activityMemory) save() error {
	if m == nil || m.path == "" || m.corrupt {
		return nil
	}
	file := activityMemoryFile{
		SchemaVersion: activityMemorySchema,
		Package:       m.packageName,
		VersionCode:   m.versionCode,
		Actions:       make([]activityMemoryAction, 0, len(m.actions)),
	}
	keys := make([]string, 0, len(m.actions))
	for key := range m.actions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		targets := m.actions[key]
		names := make([]string, 0, len(targets))
		for name, count := range targets {
			if name != "" && count > 0 {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		encoded := activityMemoryAction{Key: key, Targets: make([]activityMemoryTarget, 0, len(names))}
		for _, name := range names {
			encoded.Targets = append(encoded.Targets, activityMemoryTarget{Activity: name, Count: targets[name]})
		}
		if len(encoded.Targets) > 0 {
			file.Actions = append(file.Actions, encoded)
		}
	}
	content, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	return artifacts.WriteFileAtomic(m.path, append(content, '\n'), 0o644)
}
