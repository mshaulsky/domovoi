package modules

import (
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/mshaulsky/aqaramcp"

	"github.com/mshaulsky/domovoi/internal/config"
	"github.com/mshaulsky/domovoi/internal/container"
	"github.com/mshaulsky/domovoi/internal/model"
	"github.com/mshaulsky/domovoi/internal/source"
	"github.com/mshaulsky/domovoi/internal/source/aqara"
)

// Aqara provides the "aqara" source kind.
type Aqara struct {
	container.Passive
}

// aqaraSection is the YAML shape of an aqara source's options: the account
// to sign in with, a key from the login page, or both — the key is then used
// until the service rejects it and the account takes over.
type aqaraSection struct {
	APIKey      config.Secret `yaml:"api_key"`
	Username    config.Secret `yaml:"username"`     // the Aqara Home account: e-mail or phone
	PasswordMD5 config.Secret `yaml:"password_md5"` // the MD5 of its password, never the password
	Region      string        `yaml:"region"`       // EU, RU, US, CN, KR or SG
	Endpoint    string        `yaml:"endpoint"`     // another MCP server; defaults to Aqara's
	// automations whose runs are device events, by a part of their name
	Automations []struct {
		Match  string `yaml:"match"`
		Device string `yaml:"device"`
		Event  string `yaml:"event"`
	} `yaml:"automations"`
}

// aqaraRegions are the values the login accepts.
var aqaraRegions = []string{
	aqaramcp.RegionEU,
	aqaramcp.RegionRU,
	aqaramcp.RegionUS,
	aqaramcp.RegionCN,
	aqaramcp.RegionKR,
	aqaramcp.RegionSG,
}

// aqaraEvents are the device event kinds an automation may stand for.
var aqaraEvents = []model.EventKind{
	model.EventUnlocked,
	model.EventLocked,
	model.EventLeakStarted,
	model.EventLeakEnded,
	model.EventPoweredOn,
	model.EventPoweredOff,
	model.EventButtonPressed,
	model.EventOffline,
	model.EventOnline,
}

// Name returns the module name.
func (Aqara) Name() string {
	return "aqara"
}

// Register adds the constructor.
func (Aqara) Register(c *container.Container) error {
	return c.Sources.Add("aqara", func(cfg config.SourceSection) (source.Source, error) {
		var sec aqaraSection
		if err := cfg.Options.Decode(&sec); err != nil {
			return nil, fmt.Errorf("source %s: %w", cfg.Name, err)
		}
		if err := sec.validate(cfg.Name); err != nil {
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
		client, err := aqaramcp.New(sec.APIKey.Reveal(), sec.options(cfg.Name, log)...)
		if err != nil {
			return nil, fmt.Errorf("source %s: %w", cfg.Name, err)
		}
		// The server speaks wall-clock minutes of the home, which is where the Pi is.
		acfg := aqara.Config{Name: cfg.Name, Automations: sec.automations(), Location: time.Local}
		return aqara.New(acfg, client, log, m)
	})
}

// validate checks that the section names a key or a complete account, and
// that every automation names a device of this source and a device event.
func (s aqaraSection) validate(name string) error {
	for i, a := range s.Automations {
		switch {
		case strings.TrimSpace(a.Match) == "":
			return fmt.Errorf("automations[%d]: match is required", i)
		case model.DeviceID(a.Device).Source() != name || a.Device == name+":":
			return fmt.Errorf("automations[%d]: device %q: want %s:<id>", i, a.Device, name)
		case !slices.Contains(aqaraEvents, model.EventKind(a.Event)):
			return fmt.Errorf("automations[%d]: event %q is not a device event", i, a.Event)
		}
	}
	switch {
	case s.Username == "" && s.PasswordMD5 != "":
		return errors.New("username is required with password_md5")
	case s.APIKey == "" && s.Username == "":
		return errors.New("api_key or an account (username, password_md5, region) is required")
	case s.Username == "":
		return nil
	case s.PasswordMD5 == "":
		return errors.New("password_md5 is required with username")
	case s.Region == "":
		return errors.New("region is required with username")
	case !slices.Contains(aqaraRegions, strings.ToUpper(s.Region)):
		return fmt.Errorf("region %q is not one of %s", s.Region, strings.Join(aqaraRegions, ", "))
	}
	return nil
}

// automations converts the configured automations for the source.
func (s aqaraSection) automations() []aqara.Automation {
	var out []aqara.Automation
	for _, a := range s.Automations {
		out = append(out, aqara.Automation{
			Match:  a.Match,
			Device: model.DeviceID(a.Device),
			Event:  model.EventKind(a.Event),
		})
	}
	return out
}

// options builds the client options: the endpoint, and with an account the
// sign-in, whose every success is logged — the region and the key's length,
// never the key — which is also how the key's real lifetime gets measured.
func (s aqaraSection) options(name string, log *slog.Logger) []aqaramcp.Option {
	var opts []aqaramcp.Option
	if s.Endpoint != "" {
		opts = append(opts, aqaramcp.WithEndpoint(s.Endpoint))
	}
	if s.Username != "" {
		creds := aqaramcp.Credentials{
			Username:    s.Username.Reveal(),
			PasswordMD5: s.PasswordMD5.Reveal(),
			Region:      strings.ToUpper(s.Region),
		}
		opts = append(opts, aqaramcp.WithLogin(creds), aqaramcp.WithLoginHook(func(k aqaramcp.Key) {
			log.Info("signed in to Aqara", "source", name, "region", k.Region, "key_length", len(k.APIKey))
		}))
	}
	return opts
}
