package componenthealth

import "sync"

type Status struct {
	Name      string `json:"name"`
	Supported bool   `json:"supported"`
	Ready     bool   `json:"ready"`
	Running   bool   `json:"running"`
	State     string `json:"state"`
	Version   string `json:"version,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

func New(name string, supported, ready, running bool, version, detail string) Status {
	state := "idle"
	if !supported {
		state = "unsupported"
	} else if ready {
		state = "ready"
	} else if running {
		state = "starting"
	}
	return Status{Name: name, Supported: supported, Ready: ready, Running: running, State: state, Version: version, Detail: detail}
}

func Degraded(name string, supported bool, version, detail string) Status {
	return Status{Name: name, Supported: supported, State: "degraded", Version: version, Detail: detail}
}

type Registry struct {
	mu       sync.RWMutex
	statuses map[string]Status
}

func NewRegistry() *Registry          { return &Registry{statuses: map[string]Status{}} }
func (r *Registry) Set(status Status) { r.mu.Lock(); r.statuses[status.Name] = status; r.mu.Unlock() }
func (r *Registry) Get(name string) (Status, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	status, ok := r.statuses[name]
	return status, ok
}
func (r *Registry) Snapshot() []Status {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Status, 0, len(r.statuses))
	for _, status := range r.statuses {
		result = append(result, status)
	}
	return result
}
