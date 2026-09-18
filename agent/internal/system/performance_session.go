package system

import (
	"context"
	"sync"
	"time"
)

type PerformanceCollector interface {
	Performance(context.Context, string) (Performance, error)
}

type frameRateState struct {
	operation sync.Mutex
	mu        sync.Mutex
	view      map[string]frameSample
	surface   map[string]frameSample
}

func newFrameRateState() *frameRateState {
	return &frameRateState{view: map[string]frameSample{}, surface: map[string]frameSample{}}
}

// performanceSession owns every stateful delta baseline used by one recording
// session. Compatibility queries and Monitor clients therefore cannot shorten
// the FPS or network interval written to that session's artifacts.
type performanceSession struct {
	service      *Service
	frames       *frameRateState
	cpuMu        sync.Mutex
	cpu          map[string]packageCPUSnapshot
	networkMu    sync.Mutex
	networkAt    time.Time
	network      NetInfo
	networkError error
}

func (s *Service) NewPerformanceSession() PerformanceCollector {
	return &performanceSession{service: s, frames: newFrameRateState(), cpu: map[string]packageCPUSnapshot{}}
}

func (p *performanceSession) Performance(ctx context.Context, packageName string) (Performance, error) {
	return p.service.collectPerformance(ctx, packageName, p.cpuForPackage, func(frameContext context.Context, name string) (*float64, *FrameInfo, string, error) {
		return p.service.frameRate(frameContext, name, p.frames)
	}, p.networkForPackage)
}

func (p *performanceSession) cpuForPackage(ctx context.Context, packageName string) (CPUInfo, error) {
	p.cpuMu.Lock()
	defer p.cpuMu.Unlock()
	current, err := p.service.packageCPUSnapshot(ctx, packageName)
	if err != nil {
		return CPUInfo{}, err
	}
	previous, exists := p.cpu[packageName]
	p.cpu[packageName] = current
	if !exists {
		return cpuInfoBetween(packageCPUSnapshot{}, current), nil
	}
	return cpuInfoBetween(previous, current), nil
}

func (p *performanceSession) networkForPackage(ctx context.Context, packageName string) (NetInfo, error) {
	p.networkMu.Lock()
	defer p.networkMu.Unlock()
	now := p.service.now()
	if !p.networkAt.IsZero() && now.Sub(p.networkAt) < 5*time.Second {
		return p.network, p.networkError
	}
	current, err := p.service.NetworkForPackage(ctx, packageName)
	if err == nil && !p.networkAt.IsZero() {
		current = withNetworkRate(p.network, current, now.Sub(p.networkAt))
		if current.RxBytesPerSecond != nil && current.TxBytesPerSecond != nil {
			current.RateSampledAt = now.UTC()
		}
	}
	p.network, p.networkError = current, err
	p.networkAt = now
	return p.network, p.networkError
}

func withNetworkRate(previous, current NetInfo, elapsed time.Duration) NetInfo {
	windowMillis := elapsed.Milliseconds()
	if elapsed <= 0 || current.Rx < previous.Rx || current.Tx < previous.Tx {
		return current
	}
	rxRate := float64(current.Rx-previous.Rx) / elapsed.Seconds()
	txRate := float64(current.Tx-previous.Tx) / elapsed.Seconds()
	current.RxBytesPerSecond = &rxRate
	current.TxBytesPerSecond = &txRate
	current.RateWindowMillis = windowMillis
	return current
}
