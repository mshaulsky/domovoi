package modules

import (
	"testing"

	"github.com/mshaulsky/domovoi/internal/config"
)

func TestCoreRegister(t *testing.T) {
	c := newContainer()
	c.Config.Set(&config.Config{})
	m := &Core{}
	if err := m.Register(c); err != nil {
		t.Fatal(err)
	}
	if m.Name() != "core" {
		t.Errorf("Name = %q", m.Name())
	}
	if err := m.Start(t.Context()); err == nil {
		t.Error("Start before the core is built should fail")
	}
	if err := m.Stop(t.Context()); err != nil {
		t.Errorf("Stop before build = %v", err)
	}
	core, err := c.Core.Get()
	if err != nil {
		t.Fatal(err)
	}
	st, err := c.State.Get()
	if core == nil || err != nil || st == nil {
		t.Errorf("core or state not built: %v", err)
	}
	if err := m.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := m.Stop(t.Context()); err != nil {
		t.Errorf("Stop = %v", err)
	}
}
