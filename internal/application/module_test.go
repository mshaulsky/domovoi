package application

import "testing"

func TestEnabled(t *testing.T) {
	mods := enabled()
	seen := map[string]bool{}
	for _, m := range mods {
		if seen[m.Name()] {
			t.Errorf("module %q listed twice", m.Name())
		}
		seen[m.Name()] = true
	}
	for _, want := range []string{"tuya", "pngfile", "scenes", "core"} {
		if !seen[want] {
			t.Errorf("module %q missing", want)
		}
	}
}
