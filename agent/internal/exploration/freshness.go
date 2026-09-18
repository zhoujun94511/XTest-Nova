package exploration

import (
	"context"
	"fmt"
)

const (
	freshnessFast   = "fast"
	freshnessStrict = "strict"
)

// validateObservation keeps the ownership contract without forcing a second
// accessibility dump for every coverage-oriented action.
func (m *Manager) validateObservation(ctx context.Context, config Config, expectedActivity, expectedFingerprint string, rules Rules) (string, error) {
	if config.FreshnessMode == freshnessStrict {
		foreground, activity, hierarchy, err := m.capture(ctx)
		if err != nil {
			return "", err
		}
		if foreground != config.Package {
			return "", fmt.Errorf("target package left foreground: %s", foreground)
		}
		analysis, err := AnalyzeWithInputStrategy(hierarchy, config.Package, rules, config.Seed, config.InputStrategy)
		if err != nil {
			return "", err
		}
		fingerprint := scopedFingerprint(analysis.Fingerprint, activity)
		if fingerprint != expectedFingerprint {
			return fingerprint, fmt.Errorf("page changed before action execution")
		}
		return fingerprint, nil
	}

	foreground, activity, err := m.currentForeground(ctx)
	if err != nil {
		return "", err
	}
	if foreground != config.Package || activity != expectedActivity {
		return "", fmt.Errorf("foreground window changed before action execution: %s", activity)
	}
	return expectedFingerprint, nil
}

func (m *Manager) validateSpecialObservation(ctx context.Context, config Config, expected SpecialEvent, expectedActivity string) (string, error) {
	if config.FreshnessMode == freshnessStrict {
		foreground, activity, hierarchy, err := m.capture(ctx)
		if err != nil {
			return "", err
		}
		actual, found, err := inspectSpecialActivity(hierarchy, config.Package, foreground, activity, config.SpecialHandling)
		if err != nil {
			return "", err
		}
		if !found || actual.Kind != expected.Kind || actual.Fingerprint != expected.Fingerprint {
			return actual.Fingerprint, fmt.Errorf("special scene changed before action execution")
		}
		return actual.Fingerprint, nil
	}

	foreground, activity, err := m.currentForeground(ctx)
	if err != nil {
		return "", err
	}
	if foreground != expected.Foreground || activity != expectedActivity {
		return "", fmt.Errorf("foreground window changed before special action execution: %s", activity)
	}
	return expected.Fingerprint, nil
}
