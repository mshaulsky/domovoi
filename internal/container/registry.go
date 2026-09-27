package container

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
)

// Registry maps kind names to what provides them: source constructors,
// display constructors, scenes.
type Registry[T any] struct {
	mu    sync.Mutex
	items map[string]T
}

// Add registers an item under a kind. A kind registered twice is an error:
// two modules claiming one name is a wiring mistake, not a preference.
func (r *Registry[T]) Add(kind string, item T) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.items == nil {
		r.items = map[string]T{}
	}
	if _, dup := r.items[kind]; dup {
		return fmt.Errorf("container: kind %q registered twice", kind)
	}
	r.items[kind] = item
	return nil
}

// Get returns the item of a kind, or an error naming the known kinds.
func (r *Registry[T]) Get(kind string) (T, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.items[kind]
	if !ok {
		var zero T
		return zero, fmt.Errorf("container: unknown kind %q (known: %s)", kind, strings.Join(r.kinds(), ", "))
	}
	return item, nil
}

// Kinds lists the registered kinds, sorted.
func (r *Registry[T]) Kinds() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.kinds()
}

func (r *Registry[T]) kinds() []string {
	return slices.Sorted(maps.Keys(r.items))
}
