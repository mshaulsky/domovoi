package container

import (
	"errors"
	"strings"
	"testing"
)

func TestLazyProvide(t *testing.T) {
	var l Lazy[int]
	l.Provide(func() (int, error) { return 42, nil })
	if v, err := l.Get(); err != nil || v != 42 {
		t.Errorf("Get = %d, %v", v, err)
	}
	defer func() {
		if recover() == nil {
			t.Error("Provide after build did not panic")
		}
	}()
	l.Provide(func() (int, error) { return 1, nil })
}

func TestLazyGet(t *testing.T) {
	t.Run("builds once", func(t *testing.T) {
		var l Lazy[int]
		builds := 0
		l.Provide(func() (int, error) { builds++; return builds, nil })
		_, _ = l.Get()
		_, _ = l.Get()
		if builds != 1 {
			t.Errorf("built %d times", builds)
		}
	})
	t.Run("remembers the error", func(t *testing.T) {
		var l Lazy[int]
		boom := errors.New("boom")
		builds := 0
		l.Provide(func() (int, error) { builds++; return 0, boom })
		for range 2 {
			if _, err := l.Get(); !errors.Is(err, boom) {
				t.Errorf("Get = %v", err)
			}
		}
		if builds != 1 {
			t.Errorf("a failed build was retried %d times", builds)
		}
	})
	t.Run("cycle fails instead of deadlocking", func(t *testing.T) {
		var l Lazy[int]
		l.Provide(func() (int, error) {
			_, err := l.Get()
			return 0, err
		})
		if _, err := l.Get(); !errors.Is(err, ErrCycle) {
			t.Errorf("Get = %v, want ErrCycle", err)
		}
	})
	t.Run("not provided", func(t *testing.T) {
		var l Lazy[*int]
		if _, err := l.Get(); !errors.Is(err, ErrNotProvided) || !strings.Contains(err.Error(), "*int") {
			t.Errorf("Get = %v", err)
		}
	})
}

func TestLazySet(t *testing.T) {
	var l Lazy[string]
	l.Provide(func() (string, error) { return "built", nil })
	l.Set("fake")
	if v, _ := l.Get(); v != "fake" {
		t.Errorf("Get = %q, want the fake", v)
	}
	defer func() {
		if recover() == nil {
			t.Error("Set after build did not panic")
		}
	}()
	l.Set("too late")
}

func TestRegistryAdd(t *testing.T) {
	var r Registry[int]
	if err := r.Add("a", 1); err != nil {
		t.Fatal(err)
	}
	if err := r.Add("a", 2); err == nil || !strings.Contains(err.Error(), `"a" registered twice`) {
		t.Errorf("duplicate Add = %v", err)
	}
}

func TestRegistryGet(t *testing.T) {
	var r Registry[int]
	mustAdd(t, &r, "b", 2)
	mustAdd(t, &r, "a", 1)
	if v, err := r.Get("a"); err != nil || v != 1 {
		t.Errorf("Get(a) = %d, %v", v, err)
	}
	if _, err := r.Get("c"); err == nil || !strings.Contains(err.Error(), `unknown kind "c" (known: a, b)`) {
		t.Errorf("Get(c) = %v", err)
	}
	var empty Registry[int]
	if _, err := empty.Get("x"); err == nil {
		t.Error("Get on an empty registry should fail")
	}
}

func TestRegistryKinds(t *testing.T) {
	var r Registry[int]
	mustAdd(t, &r, "b", 2)
	mustAdd(t, &r, "a", 1)
	if got := r.Kinds(); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("Kinds = %v", got)
	}
}

func mustAdd(t *testing.T, r *Registry[int], kind string, v int) {
	t.Helper()
	if err := r.Add(kind, v); err != nil {
		t.Fatal(err)
	}
}

func TestPassive(t *testing.T) {
	var p Passive
	if err := p.Start(t.Context()); err != nil {
		t.Errorf("Start = %v", err)
	}
	if err := p.Stop(t.Context()); err != nil {
		t.Errorf("Stop = %v", err)
	}
}

func TestNew(t *testing.T) {
	var got error
	c := New(func(err error) { got = err })
	c.Fail(errors.New("boom"))
	if got == nil || got.Error() != "boom" {
		t.Errorf("Fail delivered %v", got)
	}
	New(nil).Fail(errors.New("ignored")) // a nil callback must not panic
}
