// Package pkgmeta obtains installed-package metadata from Android's own
// PackageManager through the bundled Nova Companion. It deliberately avoids
// parsing APK binary XML and resource tables inside the native Agent.
package pkgmeta

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/platform"
)

const providerAuthority = "com.openatx.xtest.popup.metadata"
const maxMetadataBytes = 1 << 20

var packagePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(?:\.[A-Za-z][A-Za-z0-9_]*)+$`)
var classPattern = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_.$]*$`)

type Service struct {
	executor platform.Executor
	timeout  time.Duration
}

type Labels struct {
	Chinese string `json:"zh"`
	English string `json:"en"`
}

type catalogEntry struct {
	PackageName string `json:"packageName"`
	Labels
}

func New(executor platform.Executor, timeout time.Duration) *Service {
	return &Service{executor: executor, timeout: timeout}
}

func (s *Service) Activities(ctx context.Context, packageName string) ([]string, error) {
	output, err := s.read(ctx, "activities", packageName)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	for _, line := range strings.Split(string(output), "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		if !classPattern.MatchString(name) {
			return nil, errors.New("invalid activity name returned by android PackageManager")
		}
		seen[normalizeActivity(packageName, name)] = struct{}{}
	}
	activities := make([]string, 0, len(seen))
	for activity := range seen {
		activities = append(activities, activity)
	}
	sort.Strings(activities)
	if len(activities) == 0 {
		return nil, errors.New("android PackageManager returned no activities")
	}
	return activities, nil
}

func (s *Service) Icon(ctx context.Context, packageName string) ([]byte, error) {
	output, err := s.read(ctx, "icon", packageName)
	if err != nil {
		return nil, err
	}
	if len(output) < 3 || output[0] != 0xff || output[1] != 0xd8 || output[2] != 0xff {
		return nil, errors.New("android PackageManager returned an invalid JPEG icon")
	}
	return output, nil
}

func (s *Service) Labels(ctx context.Context, packageName string) (Labels, error) {
	output, err := s.read(ctx, "labels", packageName)
	if err != nil {
		return Labels{}, err
	}
	var value Labels
	if err := json.Unmarshal(output, &value); err != nil {
		return Labels{}, fmt.Errorf("decode Android package labels: %w", err)
	}
	value.Chinese = strings.TrimSpace(value.Chinese)
	value.English = strings.TrimSpace(value.English)
	return value, nil
}

// Catalog obtains all launcher-visible labels in one provider call. This avoids
// starting one Android content command for every item in the application list.
func (s *Service) Catalog(ctx context.Context) (map[string]Labels, error) {
	output, err := s.readToken(ctx, "catalog", "launchable")
	if err != nil {
		return nil, err
	}
	var entries []catalogEntry
	if err := json.Unmarshal(output, &entries); err != nil {
		return nil, fmt.Errorf("decode Android package catalog: %w", err)
	}
	result := make(map[string]Labels, len(entries))
	for _, entry := range entries {
		if packagePattern.MatchString(entry.PackageName) {
			entry.Chinese = strings.TrimSpace(entry.Chinese)
			entry.English = strings.TrimSpace(entry.English)
			result[entry.PackageName] = entry.Labels
		}
	}
	return result, nil
}

func (s *Service) read(ctx context.Context, operation, packageName string) ([]byte, error) {
	if !packagePattern.MatchString(packageName) {
		return nil, errors.New("invalid android package name")
	}
	return s.readToken(ctx, operation, packageName)
}

func (s *Service) readToken(ctx context.Context, operation, token string) ([]byte, error) {
	timeout := s.timeout
	if timeout <= 0 || timeout > 10*time.Second {
		timeout = 10 * time.Second
	}
	commandContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	uri := "content://" + providerAuthority + "/" + operation + "/" + token
	output, err := s.executor.RunBytes(commandContext, "content", "read", "--uri", uri)
	if err != nil {
		return nil, fmt.Errorf("read Android package %s: %w", operation, err)
	}
	if len(output) > maxMetadataBytes {
		return nil, fmt.Errorf("android package %s exceeds %d-byte limit", operation, maxMetadataBytes)
	}
	return output, nil
}

func normalizeActivity(packageName, name string) string {
	if strings.HasPrefix(name, packageName+".") {
		return packageName + "/." + strings.TrimPrefix(name, packageName+".")
	}
	return packageName + "/" + name
}
