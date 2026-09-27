package modules

import (
	"fmt"
	"net/http"
	"time"

	"github.com/mshaulsky/domovoi/internal/config"
	"github.com/mshaulsky/domovoi/internal/container"
	"github.com/mshaulsky/domovoi/internal/source"
	"github.com/mshaulsky/domovoi/internal/source/weather"
)

// Weather provides the "weather" source kind: Open-Meteo for one location.
type Weather struct {
	container.Passive
}

// weatherSection is the YAML shape of a weather source's options. The
// coordinates are pointers so that an omitted one is an error rather than
// the Gulf of Guinea.
type weatherSection struct {
	Latitude  *float64 `yaml:"latitude"`
	Longitude *float64 `yaml:"longitude"`
	URL       string   `yaml:"url"` // another forecast endpoint (a mirror, a test server)
}

// weatherTimeout bounds one forecast request.
const weatherTimeout = 30 * time.Second

// Name returns the module name.
func (Weather) Name() string {
	return "weather"
}

// Register adds the constructor.
func (Weather) Register(c *container.Container) error {
	return c.Sources.Add("weather", func(cfg config.SourceSection) (source.Source, error) {
		var sec weatherSection
		if err := cfg.Options.Decode(&sec); err != nil {
			return nil, fmt.Errorf("source %s: %w", cfg.Name, err)
		}
		if sec.Latitude == nil || sec.Longitude == nil {
			return nil, fmt.Errorf("source %s: latitude and longitude are required", cfg.Name)
		}
		global, err := c.Config.Get()
		if err != nil {
			return nil, err
		}
		log, err := c.Logger.Get()
		if err != nil {
			return nil, err
		}
		m, err := c.Metrics.Get()
		if err != nil {
			return nil, err
		}
		src, err := weather.New(weather.Config{
			Name:      cfg.Name,
			Latitude:  *sec.Latitude,
			Longitude: *sec.Longitude,
			Location:  global.Timezone,
			BaseURL:   sec.URL,
		}, &http.Client{Timeout: weatherTimeout}, log, m)
		if err != nil {
			return nil, fmt.Errorf("source %s: %w", cfg.Name, err)
		}
		return src, nil
	})
}
