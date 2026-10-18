package modules

import (
	"io"
	"log/slog"
	"testing"

	"github.com/mshaulsky/domovoi/internal/config"
	"github.com/mshaulsky/domovoi/internal/container"
	"github.com/mshaulsky/domovoi/internal/metrics"
)

const base = "displays:\n  - kind: pngfile\n"

// parse builds a config from YAML with the given source and display
// sections appended.
func parse(t *testing.T, yaml string) config.Config {
	t.Helper()
	cfg, err := config.Parse([]byte(yaml), config.MapLookup(map[string]string{"ID": "id", "KEY": "key"}))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func newContainer() *container.Container {
	c := container.New(nil)
	c.Logger.Set(slog.New(slog.NewTextHandler(io.Discard, nil)))
	c.Metrics.Set(metrics.New())
	return c
}
