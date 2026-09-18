package recordreplay

import (
	"encoding/xml"
	"strings"
	"unicode"
)

var permissionAllowMarkers = []string{
	"allow", "while using the app", "only this time",
	"允许", "使用应用时", "仅此一次",
}

var permissionDenyMarkers = []string{
	"don't allow", "dont allow", "do not allow", "not allow", "disallow", "deny",
	"不允许", "拒绝", "禁止",
}

func isPermissionController(packageName string) bool {
	switch packageName {
	case "com.android.permissioncontroller",
		"com.google.android.permissioncontroller",
		"com.samsung.android.permissioncontroller",
		"com.android.packageinstaller",
		"com.google.android.packageinstaller",
		"com.miui.packageinstaller",
		"com.miui.securitycenter",
		"com.lbe.security.miui":
		return true
	}
	return strings.Contains(packageName, "permissioncontroller") || strings.Contains(packageName, "packageinstaller")
}

func permissionAllowPoint(document string, width, height int) (Point, bool) {
	if document == "" || width < 1 || height < 1 {
		return Point{}, false
	}
	var root hierarchyTargetNode
	if xml.Unmarshal([]byte(document), &root) != nil {
		return Point{}, false
	}
	bestArea := 2.0
	var best Point
	found := false
	var visit func(hierarchyTargetNode)
	visit = func(node hierarchyTargetNode) {
		if matchesPermissionAllow(node) {
			if bounds, ok := normalizedTargetBounds(node.Bounds, width, height); ok {
				area := (bounds.Right - bounds.Left) * (bounds.Bottom - bounds.Top)
				if area > 0 && area < bestArea {
					bestArea = area
					best = Point{X: (bounds.Left + bounds.Right) / 2, Y: (bounds.Top + bounds.Bottom) / 2}
					found = true
				}
			}
		}
		for _, child := range node.Children {
			visit(child)
		}
	}
	visit(root)
	return best, found
}

func matchesPermissionAllow(node hierarchyTargetNode) bool {
	resource := strings.ToLower(node.ResourceID)
	if strings.Contains(resource, "permission_deny") || strings.Contains(resource, "permission_do_not") {
		return false
	}
	haystack := normalizePermissionText(strings.Join([]string{node.Text, node.Description, resource}, " "))
	for _, marker := range permissionDenyMarkers {
		if strings.Contains(haystack, marker) {
			return false
		}
	}
	if strings.Contains(resource, "permission_allow") {
		return true
	}
	for _, marker := range permissionAllowMarkers {
		if marker == "allow" {
			if hasPermissionAllowWord(haystack) {
				return true
			}
			continue
		}
		if strings.Contains(haystack, marker) {
			return true
		}
	}
	return false
}

func normalizePermissionText(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		return unicode.ToLower(r)
	}, value)
}

func hasPermissionAllowWord(haystack string) bool {
	for _, word := range strings.Fields(haystack) {
		if word == "allow" {
			return true
		}
	}
	return false
}
