package modules

import (
	"testing"
)

func TestScenesRegister(t *testing.T) {
	c := newContainer()
	m := Scenes{}
	if err := m.Register(c); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Scenes.Get("overview"); err != nil {
		t.Errorf("overview not registered: %v", err)
	}
	if err := m.Register(c); err == nil {
		t.Error("registering twice should fail")
	}
	if m.Name() != "scenes" || m.Start(t.Context()) != nil || m.Stop(t.Context()) != nil {
		t.Error("passive lifecycle misbehaves")
	}
}
