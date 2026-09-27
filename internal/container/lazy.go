package container

import (
	"errors"
	"fmt"
	"sync"
)

// Lazy is a singleton built on first Get. Provide installs the builder, Set
// installs a ready value (tests use it for fakes). Singletons are built on
// one goroutine during assembly; a Get from inside a builder that leads back
// to the same Lazy is a dependency cycle and fails with ErrCycle instead of
// deadlocking.
type Lazy[T any] struct {
	mu       sync.Mutex
	build    func() (T, error)
	building bool
	built    bool
	v        T
	err      error
}

// Errors Get reports besides the builder's own.
var (
	ErrNotProvided = errors.New("container: singleton not provided")
	ErrCycle       = errors.New("container: dependency cycle while building")
)

// Provide installs the builder. Calling it after the value was built is a
// programming error and panics.
func (l *Lazy[T]) Provide(build func() (T, error)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.built || l.building {
		panic("container: Provide after the singleton was built")
	}
	l.build = build
}

// Get returns the value, building it on the first call. A build error is
// remembered and returned on every later call.
func (l *Lazy[T]) Get() (T, error) {
	l.mu.Lock()
	if l.built {
		defer l.mu.Unlock()
		return l.v, l.err
	}
	var zero T
	if l.build == nil {
		l.mu.Unlock()
		return zero, fmt.Errorf("%w (%T)", ErrNotProvided, zero)
	}
	if l.building {
		l.mu.Unlock()
		return zero, fmt.Errorf("%w (%T)", ErrCycle, zero)
	}
	l.building = true
	build := l.build
	l.mu.Unlock()

	v, err := build()

	l.mu.Lock()
	defer l.mu.Unlock()
	l.v, l.err, l.built, l.building = v, err, true, false
	return v, err
}

// Set installs a ready value. It panics if the singleton was already built,
// so a test cannot silently install a fake that nobody will see.
func (l *Lazy[T]) Set(v T) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.built || l.building {
		panic("container: Set after the singleton was built")
	}
	l.v, l.err, l.built = v, nil, true
}
