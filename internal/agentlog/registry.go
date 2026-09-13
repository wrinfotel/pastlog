package agentlog

// Registry holds the registered adapters in deterministic order.
type Registry struct {
	items []Adapter
}

func NewRegistry() *Registry { return &Registry{} }

// Register adds an adapter; registering a name again replaces the previous
// adapter in place.
func (r *Registry) Register(a Adapter) {
	for i, existing := range r.items {
		if existing.Name() == a.Name() {
			r.items[i] = a
			return
		}
	}
	r.items = append(r.items, a)
}

// Get returns the adapter with the given name.
func (r *Registry) Get(name string) (Adapter, bool) {
	for _, a := range r.items {
		if a.Name() == name {
			return a, true
		}
	}
	return nil, false
}

// Adapters returns the registered adapters in registration order.
func (r *Registry) Adapters() []Adapter {
	out := make([]Adapter, len(r.items))
	copy(out, r.items)
	return out
}
