package perflog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	novasystem "github.com/zhoujun94511/xtest-nova/agent/internal/system"
)

type MetricSummary struct {
	Count   int     `json:"count"`
	Minimum float64 `json:"minimum"`
	Maximum float64 `json:"maximum"`
	Average float64 `json:"average"`
	P50     float64 `json:"p50"`
	P90     float64 `json:"p90"`
	P95     float64 `json:"p95"`
}

type Summary struct {
	SchemaVersion  string                   `json:"schemaVersion"`
	Package        string                   `json:"package"`
	StartedAt      time.Time                `json:"startedAt"`
	EndedAt        time.Time                `json:"endedAt"`
	Rows           int                      `json:"rows"`
	FailedSamples  int                      `json:"failedSamples"`
	PartialSamples int                      `json:"partialSamples"`
	Metrics        map[string]MetricSummary `json:"metrics"`
	MetricQuality  map[string]MetricQuality `json:"metricQuality"`
	Warnings       []MetricWarning          `json:"warnings,omitempty"`
}

type MetricWarning struct {
	Metric          string  `json:"metric"`
	Code            string  `json:"code"`
	Message         string  `json:"message"`
	ValidSamples    int     `json:"validSamples"`
	TotalSamples    int     `json:"totalSamples"`
	CoveragePercent float64 `json:"coveragePercent"`
}

type MetricQuality struct {
	Measured    int `json:"measured"`
	Idle        int `json:"idle"`
	WarmingUp   int `json:"warmingUp"`
	Unsupported int `json:"unsupported"`
	Failed      int `json:"failed"`
}

type metricAccumulator struct {
	count  int
	min    float64
	max    float64
	sum    float64
	values []float64
}

type accumulator struct {
	metrics               map[string]*metricAccumulator
	quality               map[string]MetricQuality
	lastNetworkRateSample time.Time
}

func newAccumulator() *accumulator {
	return &accumulator{metrics: map[string]*metricAccumulator{}, quality: map[string]MetricQuality{}}
}

func (a *accumulator) add(name string, value float64) {
	metric := a.metrics[name]
	if metric == nil {
		metric = &metricAccumulator{min: value, max: value}
		a.metrics[name] = metric
	}
	metric.count++
	metric.sum += value
	metric.values = append(metric.values, value)
	if value < metric.min {
		metric.min = value
	}
	if value > metric.max {
		metric.max = value
	}
}

func (a *accumulator) Add(sample novasystem.Performance) {
	for name, metricState := range sample.MetricStates {
		quality := a.quality[name]
		switch metricState.State {
		case "measured":
			quality.Measured++
		case "idle":
			quality.Idle++
		case "warming_up":
			quality.WarmingUp++
		case "unsupported":
			quality.Unsupported++
		case "failed":
			quality.Failed++
		}
		a.quality[name] = quality
	}
	if state := sample.MetricStates["cpu"].State; state == "" || state == "measured" || state == "idle" {
		a.add("app_cpu_device_percent", sample.CPU.Percent)
		a.add("app_cpu_core_percent", sample.CPU.CorePercent)
		a.add("system_cpu_percent", sample.CPU.SystemPercent)
	}
	if memory := performanceMemory(sample); memory > 0 {
		a.add("memory_kb", float64(memory))
	}
	for source, metric := range map[string]string{"java heap": "memory_java_heap_kb", "native heap": "memory_native_heap_kb", "graphics": "memory_graphics_kb", "private other": "memory_private_other_kb"} {
		if value := sample.Memory[source]; value > 0 {
			a.add(metric, float64(value))
		}
	}
	if sample.FPS != nil && sample.MetricStates["fps"].State != "idle" {
		a.add("fps", *sample.FPS)
	}
	if sample.RenderFPS != nil && sample.MetricStates["render_rate"].State != "idle" {
		a.add("render_fps", *sample.RenderFPS)
	}
	if sample.Frame != nil && sample.MetricStates["jank"].State != "idle" {
		a.add("jank_count", float64(sample.Frame.JankCount))
		a.add("jank_rate_percent", sample.Frame.JankRate)
	}
	if sample.GPU != nil {
		a.add("gpu_percent", sample.GPU.Percent)
	}
	if sample.Battery.CurrentMA != nil {
		a.add("battery_current_ma", *sample.Battery.CurrentMA)
	}
	if !sample.Network.RateSampledAt.IsZero() && sample.Network.RateSampledAt.After(a.lastNetworkRateSample) {
		if sample.Network.RxBytesPerSecond != nil {
			a.add("network_rx_bytes_per_second", *sample.Network.RxBytesPerSecond)
		}
		if sample.Network.TxBytesPerSecond != nil {
			a.add("network_tx_bytes_per_second", *sample.Network.TxBytesPerSecond)
		}
		a.lastNetworkRateSample = sample.Network.RateSampledAt
	}
	a.add("battery_temperature_c", sample.Battery.Temperature)
	a.add("collection_duration_ms", float64(sample.CollectionDurationMillis))
}

func metricWarnings(rows int, metrics map[string]MetricSummary) []MetricWarning {
	if rows <= 0 {
		return nil
	}
	var result []MetricWarning
	for _, candidate := range []struct {
		metric, label string
	}{
		{"fps", "presented FPS"},
		{"jank_rate_percent", "jank rate"},
	} {
		valid := metrics[candidate.metric].Count
		coverage := 100 * float64(valid) / float64(rows)
		if valid < 30 || coverage < 10 {
			result = append(result, MetricWarning{
				Metric: candidate.metric, Code: "insufficient_window_coverage",
				Message:      candidate.label + " has too few valid windows for a reliable aggregate",
				ValidSamples: valid, TotalSamples: rows, CoveragePercent: coverage,
			})
		}
	}
	return result
}

func (a *accumulator) Quality() map[string]MetricQuality {
	result := make(map[string]MetricQuality, len(a.quality))
	for name, quality := range a.quality {
		result[name] = quality
	}
	return result
}

func (a *accumulator) Metrics() map[string]MetricSummary {
	result := make(map[string]MetricSummary, len(a.metrics))
	for name, value := range a.metrics {
		samples := append([]float64(nil), value.values...)
		sort.Float64s(samples)
		result[name] = MetricSummary{Count: value.count, Minimum: value.min, Maximum: value.max, Average: value.sum / float64(value.count), P50: percentile(samples, .50), P90: percentile(samples, .90), P95: percentile(samples, .95)}
	}
	return result
}

func percentile(sorted []float64, fraction float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	index := int(float64(len(sorted))*fraction+.999999999) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func performanceMemory(sample novasystem.Performance) int {
	if value := sample.Memory["total pss"]; value > 0 {
		return value
	}
	return sample.Memory["total"]
}

func writeJSONAtomic(path string, value any) error {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	temporary := path + ".tmp"
	if err = os.WriteFile(temporary, content, 0o644); err != nil {
		return err
	}
	if err = os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

func artifactPath(directory, name string) string {
	return filepath.ToSlash(filepath.Join(directory, name))
}
