package display

import "testing"

func TestThreeColour(t *testing.T) {
	p := ThreeColour()
	if len(p) != 3 || p[0] != White || p[1] != Black || p[2] != Red {
		t.Errorf("ThreeColour() = %v", p)
	}
}

func TestMono(t *testing.T) {
	p := Mono()
	if len(p) != 2 || p[0] != White || p[1] != Black {
		t.Errorf("Mono() = %v", p)
	}
}

func TestModeString(t *testing.T) {
	tests := []struct {
		name string
		mode Mode
		want string
	}{
		{name: "full", mode: ModeFull, want: "full"},
		{name: "fast", mode: ModeFast, want: "fast"},
		{name: "partial", mode: ModePartial, want: "partial"},
		{name: "unknown", mode: Mode(7), want: "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mode.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}
