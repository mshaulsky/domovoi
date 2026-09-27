package scene

import (
	"bytes"
	"flag"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/mshaulsky/domovoi/internal/display"
)

var update = flag.Bool("update", false, "rewrite the golden images")

func surface(w, h int, mono bool) display.Surface {
	p := display.ThreeColour()
	if mono {
		p = display.Mono()
	}
	return display.Surface{Bounds: image.Rect(0, 0, w, h), Palette: p}
}

func TestOverviewName(t *testing.T) {
	if got := (Overview{}).Name(); got != "overview" {
		t.Errorf("Name() = %q", got)
	}
}

// TestOverviewRender compares frames against golden images. Run with
// -update after an intended change and review the PNGs in testdata.
func TestOverviewRender(t *testing.T) {
	tests := []struct {
		name    string
		lang    string
		surface display.Surface
		mutate  func(v *View)
	}{
		{name: "full_ru", lang: "ru", surface: surface(800, 480, false)},
		{name: "full_en", lang: "en", surface: surface(800, 480, false)},
		{name: "mono_small", lang: "en", surface: surface(400, 300, true)},
		{
			name: "no_alerts_no_hallway", lang: "ru", surface: surface(800, 480, false),
			mutate: func(v *View) {
				v.Alerts = nil
				v.Devices = v.Devices[:5]
				v.Note = "Дома всё спокойно, влажность в норме."
			},
		},
		{
			name: "restored", lang: "ru", surface: surface(800, 480, false),
			mutate: func(v *View) {
				v.Restored = true
				v.Alerts, v.Events = nil, nil
				for i := range v.Sources {
					v.Sources[i].LastOK, v.Sources[i].Stale = time.Time{}, true
				}
			},
		},
		{
			name: "starting", lang: "en", surface: surface(800, 480, false),
			mutate: func(v *View) {
				v.Devices, v.Readings, v.Alerts, v.Events = nil, nil, nil, nil
				v.Sources = []SourceStatus{{Name: "tuya", Stale: true}}
				v.Starting = true
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := fixture(t, tt.lang)
			if tt.mutate != nil {
				tt.mutate(&v)
			}
			img, err := (Overview{}).Render(tt.surface, v)
			if err != nil {
				t.Fatal(err)
			}
			if img.Bounds() != tt.surface.Bounds {
				t.Fatalf("bounds = %v, want %v", img.Bounds(), tt.surface.Bounds)
			}
			var got bytes.Buffer
			if err := png.Encode(&got, img); err != nil {
				t.Fatal(err)
			}
			golden := filepath.Join("testdata", tt.name+".png")
			if *update {
				if err := os.WriteFile(golden, got.Bytes(), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("no golden image; run go test ./internal/scene -update: %v", err)
			}
			if !bytes.Equal(got.Bytes(), want) {
				failed := filepath.Join(t.TempDir(), tt.name+".png")
				_ = os.WriteFile(failed, got.Bytes(), 0o644)
				t.Errorf("frame differs from %s; rendered frame kept at %s", golden, failed)
			}
		})
	}
}

func TestOverviewRenderErrors(t *testing.T) {
	v := fixture(t, "en")
	_, err := (Overview{}).Render(display.Surface{Bounds: image.Rect(0, 0, 10, 10), Palette: display.Mono()[:1]}, v)
	if err == nil {
		t.Error("Render with a one-colour palette should fail")
	}
}

// TestOverviewRenderConcurrent renders on two goroutines at once, the way
// two displays aligned to the same tick do; under -race it guards the
// per-render face cache.
func TestOverviewRenderConcurrent(t *testing.T) {
	v := fixture(t, "ru")
	s := surface(800, 480, false)
	frames := make([]*image.Paletted, 2)
	var wg sync.WaitGroup
	for i := range frames {
		wg.Go(func() {
			img, err := (Overview{}).Render(s, v)
			if err != nil {
				t.Error(err)
				return
			}
			frames[i] = img
		})
	}
	wg.Wait()
	if frames[0] == nil || frames[1] == nil || !bytes.Equal(frames[0].Pix, frames[1].Pix) {
		t.Error("concurrent renders differ")
	}
}
