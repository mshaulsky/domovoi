package modules

import (
	"fmt"
	"sync"
	"time"

	epd "github.com/mshaulsky/waveshare-epd7in5b-v2"
	"github.com/mshaulsky/waveshare-epd7in5b-v2/periphconn"
	"periph.io/x/conn/v3/physic"
	"periph.io/x/host/v3"

	"github.com/mshaulsky/domovoi/internal/config"
	"github.com/mshaulsky/domovoi/internal/container"
	"github.com/mshaulsky/domovoi/internal/display"
	"github.com/mshaulsky/domovoi/internal/display/epd7in5b"
)

// EPD7in5b provides the "epd7in5b" display kind: the Waveshare 7.5" (B) V2
// panel on SPI and GPIO.
type EPD7in5b struct {
	container.Passive
}

// epd7in5bSection is the YAML shape of an epd7in5b display's options.
type epd7in5bSection struct {
	SPI      string        `yaml:"spi"`
	Pins     epd7in5bPins  `yaml:"pins"`
	Rotation int           `yaml:"rotation"`
	Border   string        `yaml:"border"`
	Timeout  time.Duration `yaml:"timeout"`
}

// epd7in5bPins names the control lines by periph GPIO name.
type epd7in5bPins struct {
	Reset string `yaml:"rst"`
	DC    string `yaml:"dc"`
	Busy  string `yaml:"busy"`
	Power string `yaml:"power"`
}

// epd7in5bSpeed is the SPI clock the driver is measured at.
const epd7in5bSpeed = 4 * physic.MegaHertz

var (
	// rotations maps the config's degrees to the controller's scan direction.
	rotations = map[int]epd.Rotation{
		0:   epd.Rotate0,
		180: epd.Rotate180,
	}

	// borders maps the config words to the border ring's colour.
	borders = map[string]epd.Border{
		"white": epd.BorderWhite,
		"black": epd.BorderBlack,
		"red":   epd.BorderRed,
	}

	// initHost loads periph's host drivers once per process.
	initHost = sync.OnceValue(func() error {
		_, err := host.Init()
		return err
	})
)

// Name returns the module name.
func (EPD7in5b) Name() string {
	return "epd7in5b"
}

// Register adds the constructor. The options are validated before anything
// is opened; under -check the display is built against a no-op panel and
// neither SPI nor GPIO is touched.
func (EPD7in5b) Register(c *container.Container) error {
	return c.Displays.Add("epd7in5b", func(cfg config.DisplaySection) (display.Display, error) {
		hat := periphconn.HATPins
		sec := epd7in5bSection{
			SPI:    "SPI0.0",
			Pins:   epd7in5bPins{Reset: hat.Reset, DC: hat.DC, Busy: hat.Busy, Power: hat.Power},
			Border: "white",
		}
		if err := cfg.Options.Decode(&sec); err != nil {
			return nil, fmt.Errorf("display %s: %w", cfg.Name, err)
		}
		rotation, ok := rotations[sec.Rotation]
		if !ok {
			return nil, fmt.Errorf("display %s: rotation %d is not 0 or 180", cfg.Name, sec.Rotation)
		}
		border, ok := borders[sec.Border]
		if !ok {
			return nil, fmt.Errorf("display %s: border %q is not white, black or red", cfg.Name, sec.Border)
		}
		if sec.Pins.Reset == "" || sec.Pins.DC == "" || sec.Pins.Busy == "" {
			return nil, fmt.Errorf("display %s: pins rst, dc and busy are required", cfg.Name)
		}
		pins := periphconn.PinConfig{Reset: sec.Pins.Reset, DC: sec.Pins.DC, Busy: sec.Pins.Busy, Power: sec.Pins.Power}
		if pins.Power == "none" {
			pins.Power = ""
		}
		log, err := c.Logger.Get()
		if err != nil {
			return nil, err
		}
		log = log.With("display", cfg.Name)
		dcfg := epd7in5b.Config{Timeout: sec.Timeout}
		if c.Check {
			d, err := epd7in5b.New(dcfg, epd7in5b.NopPanel{}, nil, log)
			if err != nil {
				return nil, fmt.Errorf("display %s: %w", cfg.Name, err)
			}
			return d, nil
		}
		if err := initHost(); err != nil {
			return nil, fmt.Errorf("display %s: periph host: %w", cfg.Name, err)
		}
		opts := []epd.Option{epd.WithRotation(rotation), epd.WithBorder(border)}
		dev, port, err := periphconn.Open(sec.SPI, epd7in5bSpeed, pins, opts...)
		if err != nil {
			return nil, fmt.Errorf("display %s: %w", cfg.Name, err)
		}
		d, err := epd7in5b.New(dcfg, dev, port, log)
		if err != nil {
			port.Close()
			return nil, fmt.Errorf("display %s: %w", cfg.Name, err)
		}
		return d, nil
	})
}
