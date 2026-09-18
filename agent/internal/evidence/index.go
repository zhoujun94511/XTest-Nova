package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/artifacts"
	"github.com/zhoujun94511/xtest-nova/agent/internal/execution"
)

const SchemaVersion = "xtest-nova-evidence/v1"

type Item struct {
	Type       string    `json:"type"`
	Path       string    `json:"path"`
	SHA256     string    `json:"sha256"`
	SizeBytes  int64     `json:"sizeBytes"`
	StepID     string    `json:"stepId,omitempty"`
	CapturedAt time.Time `json:"capturedAt"`
}

type Index struct {
	SchemaVersion   string             `json:"schemaVersion"`
	Identity        execution.Identity `json:"identity"`
	Package         string             `json:"package"`
	CaseFingerprint string             `json:"caseFingerprint,omitempty"`
	CreatedAt       time.Time          `json:"createdAt"`
	Items           []Item             `json:"items"`
	Errors          []string           `json:"errors,omitempty"`
}

// Build indexes only regular files under root. This prevents an evidence path
// from escaping its attempt directory through traversal or a symlink.
func Build(root, packageName, caseFingerprint string, identity execution.Identity, paths []string) Index {
	return BuildForPublication(root, root, packageName, caseFingerprint, identity, paths)
}

// BuildForPublication hashes files under root while recording their paths as
// they will appear under publishedRoot after a directory transaction commits.
func BuildForPublication(root, publishedRoot, packageName, caseFingerprint string, identity execution.Identity, paths []string) Index {
	identity.OwnerToken = ""
	index := Index{SchemaVersion: SchemaVersion, Identity: identity, Package: packageName, CaseFingerprint: caseFingerprint, CreatedAt: time.Now().UTC(), Items: []Item{}}
	canonicalRoot, rootErr := filepath.Abs(root)
	if rootErr != nil {
		index.Errors = append(index.Errors, rootErr.Error())
		return index
	}
	canonicalPublishedRoot, publishedErr := filepath.Abs(publishedRoot)
	if publishedErr != nil {
		index.Errors = append(index.Errors, publishedErr.Error())
		return index
	}
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		absolute, err := filepath.Abs(path)
		if err != nil || (absolute != canonicalRoot && !strings.HasPrefix(absolute, canonicalRoot+string(filepath.Separator))) {
			index.Errors = append(index.Errors, "evidence path is outside attempt directory: "+path)
			continue
		}
		info, err := os.Lstat(absolute)
		if err != nil || !info.Mode().IsRegular() {
			if err == nil {
				err = errors.New("evidence is not a regular file")
			}
			index.Errors = append(index.Errors, filepath.Base(path)+": "+err.Error())
			continue
		}
		digest, err := digestFile(absolute)
		if err != nil {
			index.Errors = append(index.Errors, filepath.Base(path)+": "+err.Error())
			continue
		}
		relative, err := filepath.Rel(canonicalRoot, absolute)
		if err != nil {
			index.Errors = append(index.Errors, filepath.Base(path)+": "+err.Error())
			continue
		}
		publishedPath := filepath.Join(canonicalPublishedRoot, relative)
		index.Items = append(index.Items, Item{Type: classify(path), Path: filepath.ToSlash(publishedPath), SHA256: digest, SizeBytes: info.Size(), CapturedAt: info.ModTime().UTC()})
	}
	sort.Slice(index.Items, func(i, j int) bool { return index.Items[i].Path < index.Items[j].Path })
	return index
}

func Write(path string, index Index) error {
	if err := Validate(index); err != nil {
		return err
	}
	content, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	return artifacts.WriteFileAtomic(path, append(content, '\n'), 0o644)
}

// Validate rejects incomplete indexes so callers cannot publish Errors as a
// successful evidence result.
func Validate(index Index) error {
	if len(index.Errors) > 0 {
		return errors.New("evidence index: " + strings.Join(index.Errors, "; "))
	}
	return nil
}

func digestFile(path string) (digest string, err error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() {
		if closeErr := file.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func classify(path string) string {
	name := strings.ToLower(filepath.Base(path))
	switch {
	case strings.HasSuffix(name, ".png"), strings.HasSuffix(name, ".jpg"), strings.HasSuffix(name, ".jpeg"):
		return "screenshot"
	case strings.Contains(name, "anr"):
		return "anr"
	case strings.Contains(name, "crash"), strings.Contains(name, "exit"):
		return "crash"
	case strings.HasSuffix(name, ".jsonl"), strings.Contains(name, "event"):
		return "timeline"
	case strings.Contains(name, "perf"), strings.Contains(name, "summary"):
		return "performance"
	default:
		return "artifact"
	}
}
