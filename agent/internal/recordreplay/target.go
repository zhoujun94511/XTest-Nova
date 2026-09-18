package recordreplay

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

type hierarchyTargetNode struct {
	Text        string                `xml:"text,attr"`
	Class       string                `xml:"class,attr"`
	ResourceID  string                `xml:"resource-id,attr"`
	Description string                `xml:"content-desc,attr"`
	Bounds      string                `xml:"bounds,attr"`
	Children    []hierarchyTargetNode `xml:"node"`
}

func relocateTarget(document string, target ActionTarget, width, height int) (Point, bool) {
	if document == "" || width < 1 || height < 1 {
		return Point{}, false
	}
	var root hierarchyTargetNode
	if xml.Unmarshal([]byte(document), &root) != nil {
		return Point{}, false
	}
	original := Point{X: (target.Bounds.Left + target.Bounds.Right) / 2, Y: (target.Bounds.Top + target.Bounds.Bottom) / 2}
	bestScore, bestDistance := 0, math.MaxFloat64
	var best Point
	var visit func(hierarchyTargetNode)
	visit = func(node hierarchyTargetNode) {
		bounds, ok := normalizedTargetBounds(node.Bounds, width, height)
		if ok {
			score := 0
			if target.ResourceID != "" && node.ResourceID == target.ResourceID {
				score += 100
			}
			if target.Text != "" && node.Text == target.Text {
				score += 40
			}
			if target.ContentDescription != "" && node.Description == target.ContentDescription {
				score += 40
			}
			if target.ClassName != "" && node.Class == target.ClassName {
				score += 10
			}
			center := Point{X: (bounds.Left + bounds.Right) / 2, Y: (bounds.Top + bounds.Bottom) / 2}
			distance := math.Hypot(center.X-original.X, center.Y-original.Y)
			minimum := 40
			if target.ResourceID != "" {
				minimum = 100
			}
			if score >= minimum && (score > bestScore || score == bestScore && distance < bestDistance) {
				bestScore, bestDistance, best = score, distance, center
			}
		}
		for _, child := range node.Children {
			visit(child)
		}
	}
	visit(root)
	return best, bestScore > 0
}

var targetBoundsPattern = regexp.MustCompile(`^\[(\d+),(\d+)]\[(\d+),(\d+)]$`)

func targetAt(document string, x, y float64, width, height int) *ActionTarget {
	if document == "" || width < 1 || height < 1 || !validPoint(Point{X: x, Y: y}) {
		return nil
	}
	var root hierarchyTargetNode
	if xml.Unmarshal([]byte(document), &root) != nil {
		return nil
	}
	var selected *ActionTarget
	selectedArea := 2.0
	var visit func(hierarchyTargetNode)
	visit = func(node hierarchyTargetNode) {
		if bounds, ok := normalizedTargetBounds(node.Bounds, width, height); ok && bounds.contains(Point{X: x, Y: y}) {
			area := (bounds.Right - bounds.Left) * (bounds.Bottom - bounds.Top)
			if area <= selectedArea && (node.ResourceID != "" || node.Text != "" || node.Description != "" || node.Class != "") {
				digest := sha256.Sum256([]byte(document))
				selected = &ActionTarget{
					ResourceID: sanitizeTargetValue(node.ResourceID), Text: sanitizeTargetValue(node.Text), ContentDescription: sanitizeTargetValue(node.Description),
					ClassName: sanitizeTargetValue(node.Class), Bounds: bounds, ObservationID: "hierarchy:" + hex.EncodeToString(digest[:8]),
				}
				selectedArea = area
			}
		}
		for _, child := range node.Children {
			visit(child)
		}
	}
	visit(root)
	return selected
}

func normalizedTargetBounds(value string, width, height int) (Bounds, bool) {
	match := targetBoundsPattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(match) != 5 {
		return Bounds{}, false
	}
	values := make([]int, 4)
	for index := range values {
		parsed, err := strconv.Atoi(match[index+1])
		if err != nil {
			return Bounds{}, false
		}
		values[index] = parsed
	}
	bounds := Bounds{Left: float64(values[0]) / float64(width), Top: float64(values[1]) / float64(height), Right: float64(values[2]) / float64(width), Bottom: float64(values[3]) / float64(height)}
	if !bounds.valid() {
		return Bounds{}, false
	}
	return bounds, true
}

func sanitizeTargetValue(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\x00", ""))
	if utf8.RuneCountInString(value) <= 256 {
		return value
	}
	runes := []rune(value)
	return string(runes[:256])
}
