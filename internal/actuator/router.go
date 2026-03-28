package actuator

import (
	"context"
	"sync"
)

// RouterBackend dispatches directives to service-specific backends.
type RouterBackend struct {
	mu             sync.RWMutex
	routes         map[string]Backend
	defaultBackend Backend
	fallback       Backend
}

func NewRouterBackend(defaultBackend Backend) *RouterBackend {
	return &RouterBackend{
		routes:         make(map[string]Backend),
		defaultBackend: defaultBackend,
		fallback:       &LogOnlyBackend{},
	}
}

func (r *RouterBackend) AddRoute(serviceID string, backend Backend) {
	if serviceID == "" || backend == nil {
		return
	}

	r.mu.Lock()
	r.routes[serviceID] = backend
	r.mu.Unlock()
}

func (r *RouterBackend) Execute(ctx context.Context, snap DirectiveSnapshot) error {
	r.mu.RLock()
	backend := r.routes[snap.ServiceID]
	defaultBackend := r.defaultBackend
	fallback := r.fallback
	r.mu.RUnlock()

	if backend != nil {
		return backend.Execute(ctx, snap)
	}
	if defaultBackend != nil {
		return defaultBackend.Execute(ctx, snap)
	}
	return fallback.Execute(ctx, snap)
}
