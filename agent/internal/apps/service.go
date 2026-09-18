package apps

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/pkgmeta"
	"github.com/zhoujun94511/xtest-nova/agent/internal/platform"
)

var packageNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(?:\.[A-Za-z][A-Za-z0-9_]*)+$`)
var versionCodePattern = regexp.MustCompile(`versionCode=(\d+)`)
var systemFlagPattern = regexp.MustCompile(`(?m)(?:pkgFlags|flags)=\[[^]]*\bSYSTEM\b`)

type Package struct {
	PackageName  string `json:"packageName"`
	Name         string `json:"name"`
	ChineseName  string `json:"chineseName,omitempty"`
	EnglishName  string `json:"englishName,omitempty"`
	APKPath      string `json:"apkPath,omitempty"`
	MainActivity string `json:"mainActivity,omitempty"`
	VersionName  string `json:"versionName,omitempty"`
	VersionCode  int64  `json:"versionCode,omitempty"`
	System       bool   `json:"system"`
}
type Service struct {
	executor platform.Executor
	timeout  time.Duration
	metadata interface {
		Icon(context.Context, string) ([]byte, error)
		Labels(context.Context, string) (pkgmeta.Labels, error)
		Catalog(context.Context) (map[string]pkgmeta.Labels, error)
	}
}

func New(executor platform.Executor, timeout time.Duration, metadata ...interface {
	Icon(context.Context, string) ([]byte, error)
	Labels(context.Context, string) (pkgmeta.Labels, error)
	Catalog(context.Context) (map[string]pkgmeta.Labels, error)
}) *Service {
	service := &Service{executor: executor, timeout: timeout}
	if len(metadata) > 0 {
		service.metadata = metadata[0]
	}
	return service
}
func (s *Service) run(ctx context.Context, name string, args ...string) (string, error) {
	c, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.executor.Run(c, name, args...)
}
func validPackage(name string) error {
	if !packageNamePattern.MatchString(name) {
		return fmt.Errorf("invalid android package name")
	}
	return nil
}
func (s *Service) List(ctx context.Context, includeSystem bool) ([]Package, error) {
	args := []string{"list", "packages", "-f"}
	if !includeSystem {
		args = append(args, "-3")
	}
	out, err := s.run(ctx, "pm", args...)
	if err != nil {
		return nil, err
	}
	systemPackages := map[string]struct{}{}
	if includeSystem {
		systemOutput, systemErr := s.run(ctx, "pm", "list", "packages", "-s")
		if systemErr != nil {
			return nil, fmt.Errorf("list Android system packages: %w", systemErr)
		}
		for _, line := range strings.Split(systemOutput, "\n") {
			name := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "package:"))
			if packageNamePattern.MatchString(name) {
				systemPackages[name] = struct{}{}
			}
		}
	}
	result := make([]Package, 0)
	labels := map[string]pkgmeta.Labels{}
	if s.metadata != nil {
		if values, metadataErr := s.metadata.Catalog(ctx); metadataErr == nil {
			labels = values
		}
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "package:"))
		if line == "" {
			continue
		}
		item := Package{}
		if i := strings.LastIndex(line, "="); i > 0 {
			item.APKPath = line[:i]
			item.PackageName = line[i+1:]
		} else {
			item.PackageName = line
		}
		if packageNamePattern.MatchString(item.PackageName) {
			applyLabels(&item, labels[item.PackageName])
			_, item.System = systemPackages[item.PackageName]
			result = append(result, item)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name == result[j].Name {
			return result[i].PackageName < result[j].PackageName
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}
func (s *Service) Info(ctx context.Context, name string) (Package, error) {
	if err := validPackage(name); err != nil {
		return Package{}, err
	}
	pathOut, err := s.run(ctx, "pm", "path", name)
	if err != nil {
		return Package{}, err
	}
	path := strings.TrimPrefix(strings.Split(strings.TrimSpace(pathOut), "\n")[0], "package:")
	if path == pathOut || path == "" {
		return Package{}, fmt.Errorf("package %q not found", name)
	}
	dump, _ := s.run(ctx, "dumpsys", "package", name)
	info := Package{PackageName: name, Name: name, APKPath: path}
	info.System = systemFlagPattern.MatchString(dump)
	if s.metadata != nil {
		if labels, metadataErr := s.metadata.Labels(ctx, name); metadataErr == nil {
			applyLabels(&info, labels)
		}
	}
	for _, line := range strings.Split(dump, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "versionName=") {
			info.VersionName = strings.TrimPrefix(line, "versionName=")
			break
		}
	}
	if m := versionCodePattern.FindStringSubmatch(dump); len(m) == 2 {
		info.VersionCode, _ = strconv.ParseInt(m[1], 10, 64)
	}
	activity, _ := s.run(ctx, "cmd", "package", "resolve-activity", "--brief", name)
	parts := strings.Split(strings.TrimSpace(activity), "/")
	if len(parts) == 2 {
		info.MainActivity = parts[1]
	}
	return info, nil
}

func applyLabels(item *Package, labels pkgmeta.Labels) {
	item.ChineseName = strings.TrimSpace(labels.Chinese)
	item.EnglishName = strings.TrimSpace(labels.English)
	item.Name = item.ChineseName
	if item.Name == "" {
		item.Name = item.EnglishName
	}
	if item.Name == "" {
		item.Name = item.PackageName
	}
}
func (s *Service) Icon(ctx context.Context, name string) ([]byte, error) {
	if err := validPackage(name); err != nil {
		return nil, err
	}
	if s.metadata == nil {
		return nil, fmt.Errorf("android package metadata service unavailable")
	}
	return s.metadata.Icon(ctx, name)
}
func (s *Service) Launch(ctx context.Context, name string) (string, error) {
	_, component, err := s.launcher(ctx, name)
	if err != nil {
		return "", err
	}
	return s.run(ctx, "am", "start", "-W", "-n", component)
}
func (s *Service) Session(ctx context.Context, name string) (Package, string, error) {
	info, component, err := s.launcher(ctx, name)
	if err != nil {
		return Package{}, "", err
	}
	output, err := s.run(ctx, "am", "start", "-W", "-S", "-n", component)
	return info, output, err
}
func (s *Service) launcher(ctx context.Context, name string) (Package, string, error) {
	info, err := s.Info(ctx, name)
	if err != nil {
		return Package{}, "", err
	}
	if info.MainActivity == "" {
		return info, "", fmt.Errorf("package has no launcher activity")
	}
	activity := info.MainActivity
	if !strings.Contains(activity, ".") {
		activity = "." + activity
	}
	return info, name + "/" + activity, nil
}
func (s *Service) Stop(ctx context.Context, name string) error {
	if err := validPackage(name); err != nil {
		return err
	}
	_, err := s.run(ctx, "am", "force-stop", name)
	return err
}
func (s *Service) Uninstall(ctx context.Context, name string, keepData bool) (string, error) {
	if err := validPackage(name); err != nil {
		return "", err
	}
	args := []string{"uninstall"}
	if keepData {
		args = append(args, "-k")
	}
	args = append(args, name)
	return s.run(ctx, "pm", args...)
}
func (s *Service) Install(ctx context.Context, path string, replace bool) (string, error) {
	if !strings.HasPrefix(path, "/data/local/tmp/") {
		return "", fmt.Errorf("APK must be staged under /data/local/tmp")
	}
	args := []string{"install"}
	if replace {
		args = append(args, "-r")
	}
	args = append(args, path)
	return s.run(ctx, "pm", args...)
}
