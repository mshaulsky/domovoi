package modules

import (
	"fmt"
	"strings"

	"github.com/mshaulsky/tuyacloud"

	"github.com/mshaulsky/domovoi/internal/config"
	"github.com/mshaulsky/domovoi/internal/container"
	"github.com/mshaulsky/domovoi/internal/source"
	"github.com/mshaulsky/domovoi/internal/source/tuya"
)

// Tuya provides the "tuya" source kind.
type Tuya struct {
	container.Passive
}

// tuyaSection is the YAML shape of a tuya source's options.
type tuyaSection struct {
	AccessID  config.Secret `yaml:"access_id"`
	AccessKey config.Secret `yaml:"access_key"`
	Region    string        `yaml:"region"`
}

// regions maps the config words to the client's data centres.
var regions = map[string]tuyacloud.Region{
	"eu": tuyacloud.RegionEurope,
	"us": tuyacloud.RegionAmerica,
	"cn": tuyacloud.RegionChina,
	"in": tuyacloud.RegionIndia,
}

// Name returns the module name.
func (Tuya) Name() string {
	return "tuya"
}

// Register adds the constructor.
func (Tuya) Register(c *container.Container) error {
	return c.Sources.Add("tuya", func(cfg config.SourceSection) (source.Source, error) {
		var sec tuyaSection
		if err := cfg.Options.Decode(&sec); err != nil {
			return nil, fmt.Errorf("source %s: %w", cfg.Name, err)
		}
		if sec.AccessID == "" || sec.AccessKey == "" {
			return nil, fmt.Errorf("source %s: access_id and access_key are required", cfg.Name)
		}
		region, ok := regions[strings.ToLower(sec.Region)]
		if sec.Region != "" && !ok {
			return nil, fmt.Errorf("source %s: unknown region %q (eu, us, cn, in)", cfg.Name, sec.Region)
		}
		opts := []tuyacloud.Option{}
		if ok {
			opts = append(opts, tuyacloud.WithRegion(region))
		}
		client, err := tuyacloud.New(sec.AccessID.Reveal(), sec.AccessKey.Reveal(), opts...)
		if err != nil {
			return nil, fmt.Errorf("source %s: %w", cfg.Name, err)
		}
		log, err := c.Logger.Get()
		if err != nil {
			return nil, err
		}
		m, err := c.Metrics.Get()
		if err != nil {
			return nil, err
		}
		return tuya.New(tuya.Config{Name: cfg.Name}, client, log, m)
	})
}
