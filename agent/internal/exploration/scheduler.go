package exploration

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"strings"
)

const (
	maxNonScrollSelectionsBeforeScroll = 2
	maxStableScrollAttempts            = 3
)

var externalRiskMarkers = []string{
	"cloud", "sync", "account", "sign in", "login", "share", "backup", "drive",
	"云", "同步", "账号", "账户", "登录", "分享", "备份", "网盘",
}

func selectAction(node *graphNode, seed int64, enableScroll, recoverPopups bool) (Action, bool) {
	return selectActionBlocked(node, seed, enableScroll, recoverPopups, nil)
}

func selectActionBlocked(node *graphNode, seed int64, enableScroll, recoverPopups bool, blocked func(Action) bool) (Action, bool) {
	candidates := make([]Action, 0, len(node.analysis.Actions))
	for _, action := range node.analysis.Actions {
		if !node.tried[action.ID] && (action.Type != "swipe" || enableScroll) && (blocked == nil || !blocked(action)) {
			candidates = append(candidates, action)
		}
	}
	if recoverPopups {
		recovery := make([]Action, 0, len(candidates))
		for _, action := range candidates {
			if action.Recovery {
				recovery = append(recovery, action)
			}
		}
		if len(recovery) > 0 {
			candidates = recovery
		}
	}
	if node.nonScrollSelections >= maxNonScrollSelectionsBeforeScroll {
		scrolls := make([]Action, 0, len(candidates))
		for _, action := range candidates {
			if action.Type == "swipe" {
				scrolls = append(scrolls, action)
			}
		}
		if len(scrolls) > 0 {
			candidates = scrolls
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		leftWeight := actionWeightForNode(node, candidates[i], recoverPopups)
		rightWeight := actionWeightForNode(node, candidates[j], recoverPopups)
		leftRank := weightedActionRank(seed+int64(len(node.tried)), candidates[i].ID, leftWeight)
		rightRank := weightedActionRank(seed+int64(len(node.tried)), candidates[j].ID, rightWeight)
		return leftRank < rightRank
	})
	if len(candidates) == 0 {
		return Action{}, false
	}
	return candidates[0], true
}

func actionWeightForNode(node *graphNode, action Action, recoverPopups bool) float64 {
	if recoverPopups && action.Recovery {
		return 1000
	}
	weight := 100.0
	switch action.Type {
	case "swipe":
		weight = 80
		if node.nonScrollSelections >= maxNonScrollSelectionsBeforeScroll {
			weight = 220
		}
	case "input":
		weight = 45
	case "back":
		weight = 25
	}
	label := strings.ToLower(strings.Join([]string{action.Description, action.ResourceID, action.Text}, " "))
	for _, marker := range externalRiskMarkers {
		if strings.Contains(label, marker) {
			weight *= 0.25
			break
		}
	}
	return weight / math.Sqrt(float64(node.actionAttempts[action.ID]+1))
}

func weightedActionRank(seed int64, id string, weight float64) float64 {
	value := actionRank(seed, id)
	uniform := (float64(value>>11) + 1) / (float64(uint64(1)<<53) + 1)
	return -math.Log(uniform) / math.Max(weight, 0.001)
}

func actionRank(seed int64, id string) uint64 {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", seed, id)))
	return binary.BigEndian.Uint64(sum[:8])
}
