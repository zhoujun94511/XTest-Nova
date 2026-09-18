package runtimebundle

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
)

const SchemaVersion = "xtest-nova-runtime-bundle/v1"

// assets contains release-signed runtime components. Validation fixtures and
// target applications are intentionally excluded.
//
//go:embed assets/manifest.json assets/xtest-nova-runner.jar assets/xtest-nova-companion.apk assets/xtest-nova-uiautomator-host.apk assets/xtest-nova-uiautomator-test.apk
var assets embed.FS

var digestPattern = regexp.MustCompile(`^[A-Fa-f0-9]{64}$`)

type Component struct {
	Name              string `json:"name"`
	File              string `json:"file"`
	Kind              string `json:"kind"`
	Package           string `json:"package,omitempty"`
	Target            string `json:"target"`
	Size              int64  `json:"size"`
	SHA256            string `json:"sha256"`
	VersionCode       int64  `json:"versionCode,omitempty"`
	VersionName       string `json:"versionName,omitempty"`
	CertificateSHA256 string `json:"certificateSha256,omitempty"`
}

type Manifest struct {
	SchemaVersion string      `json:"schemaVersion"`
	Components    []Component `json:"components"`
}

type Embedded struct{}

func (Embedded) Components() ([]Component, error) {
	data, err := assets.ReadFile("assets/manifest.json")
	if err != nil {
		return nil, err
	}
	var manifest Manifest
	if err = json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("decode runtime bundle manifest: %w", err)
	}
	if manifest.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("unsupported runtime bundle schema %q", manifest.SchemaVersion)
	}
	expected := map[string]string{"runner": "jar", "companion": "apk", "uiautomatorHost": "apk", "uiautomatorTest": "apk"}
	seen := map[string]bool{}
	for index := range manifest.Components {
		component := &manifest.Components[index]
		kind, ok := expected[component.Name]
		if !ok || seen[component.Name] {
			return nil, fmt.Errorf("unexpected or duplicate runtime component %q", component.Name)
		}
		seen[component.Name] = true
		if component.Kind != kind || path.Base(component.File) != component.File || component.Size < 1 || !digestPattern.MatchString(component.SHA256) {
			return nil, fmt.Errorf("invalid runtime component metadata for %q", component.Name)
		}
		if kind == "apk" && (component.Package == "" || component.VersionCode < 1 || !digestPattern.MatchString(component.CertificateSHA256)) {
			return nil, fmt.Errorf("incomplete APK metadata for %q", component.Name)
		}
		payload, readErr := Embedded{}.Data(component.File)
		if readErr != nil {
			return nil, readErr
		}
		digest := sha256.Sum256(payload)
		if int64(len(payload)) != component.Size || !equalHex(component.SHA256, hex.EncodeToString(digest[:])) {
			return nil, fmt.Errorf("runtime component digest mismatch for %q", component.Name)
		}
	}
	if len(seen) != len(expected) {
		return nil, errors.New("runtime bundle is incomplete")
	}
	return append([]Component(nil), manifest.Components...), nil
}

func (Embedded) Data(file string) ([]byte, error) {
	if path.Base(file) != file {
		return nil, errors.New("invalid runtime component file")
	}
	data, err := assets.ReadFile("assets/" + file)
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), data...), nil
}

func equalHex(left, right string) bool {
	leftBytes, leftErr := hex.DecodeString(left)
	rightBytes, rightErr := hex.DecodeString(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	return string(leftBytes) == string(rightBytes)
}
