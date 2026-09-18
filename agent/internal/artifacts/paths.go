package artifacts

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const Root = "/sdcard/xtest-nova"
const MaxSessionsPerKind = 200

var timestampPattern = regexp.MustCompile(`^\d{8}_\d{6}$`)
var sessionDirectoryPattern = regexp.MustCompile(`^\d{8}_\d{6}(?:_\d+)?$`)

func PackageKind(root, packageName, kind string) string {
	return filepath.Join(root, packageName, kind)
}

func NewSession(root, packageName, kind string, now time.Time) (string, error) {
	return newNamedSession(root, packageName, kind, now.Format("20060102_150405"))
}

// DeviceTimestamp follows the device's civil time. Android Go binaries can
// otherwise format time in UTC even when the device UI uses another timezone.
func DeviceTimestamp() string {
	value, err := exec.Command("/system/bin/date", "+%Y%m%d_%H%M%S").Output()
	if candidate := strings.TrimSpace(string(value)); err == nil && timestampPattern.MatchString(candidate) {
		return candidate
	}
	return time.Now().Format("20060102_150405")
}

func NewDeviceSession(root, packageName, kind string) (string, error) {
	return newNamedSession(root, packageName, kind, DeviceTimestamp())
}

func NewDeviceSessionTransaction(root, packageName, kind string) (*DirectoryTransaction, error) {
	return newNamedSessionTransaction(root, packageName, kind, DeviceTimestamp())
}

func NewSessionTransaction(root, packageName, kind string, now time.Time) (*DirectoryTransaction, error) {
	return newNamedSessionTransaction(root, packageName, kind, now.Format("20060102_150405"))
}

func newNamedSessionTransaction(root, packageName, kind, name string) (*DirectoryTransaction, error) {
	base := PackageKind(root, packageName, kind)
	if err := os.MkdirAll(base, 0o755); err != nil {
		return nil, err
	}
	for index := 1; ; index++ {
		candidate := filepath.Join(base, name)
		if index > 1 {
			candidate = filepath.Join(base, fmt.Sprintf("%s_%d", name, index))
		}
		reserve := candidate + ".reserve"
		file, reserveErr := os.OpenFile(reserve, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if reserveErr != nil {
			if os.IsExist(reserveErr) {
				continue
			}
			return nil, reserveErr
		}
		if closeErr := file.Close(); closeErr != nil {
			_ = os.Remove(reserve)
			return nil, closeErr
		}
		transaction, err := BeginDirectory(candidate)
		if err == nil {
			transaction.reserve = reserve
			return transaction, nil
		}
		_ = os.Remove(reserve)
		if !os.IsExist(err) {
			return nil, err
		}
	}
}

func newNamedSession(root, packageName, kind, name string) (string, error) {
	base := PackageKind(root, packageName, kind)
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", err
	}
	for index := 1; ; index++ {
		candidate := filepath.Join(base, name)
		if index > 1 {
			candidate = filepath.Join(base, fmt.Sprintf("%s_%d", name, index))
		}
		err := os.Mkdir(candidate, 0o755)
		if err == nil {
			return candidate, PruneSessions(base, MaxSessionsPerKind, candidate)
		}
		if !os.IsExist(err) {
			return "", err
		}
	}
}

func PruneSessions(base string, limit int, keep string) error {
	if limit < 1 {
		return fmt.Errorf("session limit must be positive")
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && sessionDirectoryPattern.MatchString(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	remaining := len(names)
	for _, name := range names {
		if remaining <= limit {
			break
		}
		path := filepath.Join(base, name)
		if filepath.Clean(path) == filepath.Clean(keep) {
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			return err
		}
		remaining--
	}
	return nil
}
