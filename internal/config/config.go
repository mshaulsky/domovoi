package config

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// Config is the file layer, parsed, resolved and validated.
type Config struct {
	Timezone *time.Location
	Language string
	Sources  []SourceSection
	Displays []DisplaySection
}

// SourceSection is one configured source instance.
type SourceSection struct {
	Kind       string
	Name       string        // prefix of the source's device IDs; defaults to Kind
	Interval   time.Duration // between polls
	StaleAfter time.Duration // without a successful poll, the source counts as stale
	Options    Options       // kind-specific keys, decoded by the owning package's glue
}

// DisplaySection is one configured display instance.
type DisplaySection struct {
	Kind      string
	Name      string // defaults to Kind; the handle commands address
	Scenes    []string
	Tick      time.Duration // render cadence
	FullEvery time.Duration // at most this long between full refreshes
	Language  string        // defaults to the global language
	Options   Options
}

// Options carries the keys of a section that the common head does not know.
// Decode is strict: a key the target struct lacks is an error, so a typo in
// a credential name never passes silently.
type Options struct {
	node *yaml.Node
}

// Defaults applied where the file is silent.
const (
	DefaultInterval  = time.Minute
	DefaultTick      = 5 * time.Minute
	DefaultFullEvery = time.Hour
	DefaultLanguage  = "en"
	// staleFactor times the interval, but never less than minStaleAfter.
	staleFactor   = 3
	minStaleAfter = 5 * time.Minute
)

var (
	placeholderRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	// nameRe keeps names usable as identifiers and as DeviceID prefixes: no
	// colon, no whitespace, nothing a shell or a URL would trip on.
	nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

	sourceHead  = []string{"kind", "name", "interval", "stale_after"}
	displayHead = []string{"kind", "name", "scenes", "tick", "full_every", "language"}
)

// file is the YAML shape of the document.
type file struct {
	Timezone string    `yaml:"timezone"`
	Language string    `yaml:"language"`
	Sources  []section `yaml:"sources"`
	Displays []section `yaml:"displays"`
}

// section keeps the raw mapping of one list entry; the head is split off
// later, once we know whether it is a source or a display.
type section struct {
	node yaml.Node
}

type sourceHeadFields struct {
	Kind       string        `yaml:"kind"`
	Name       string        `yaml:"name"`
	Interval   time.Duration `yaml:"interval"`
	StaleAfter time.Duration `yaml:"stale_after"`
}

type displayHeadFields struct {
	Kind      string        `yaml:"kind"`
	Name      string        `yaml:"name"`
	Scenes    []string      `yaml:"scenes"`
	Tick      time.Duration `yaml:"tick"`
	FullEvery time.Duration `yaml:"full_every"`
	Language  string        `yaml:"language"`
}

// Load reads and parses a configuration file.
func Load(path string, lookup Lookup) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("config: read %s: %w", path, err)
	}
	cfg, err := Parse(data, lookup)
	if err != nil {
		return Config{}, fmt.Errorf("config: %s: %w", path, err)
	}
	return cfg, nil
}

// Parse parses a YAML document, resolves its placeholders and validates it.
func Parse(data []byte, lookup Lookup) (Config, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return Config{}, fmt.Errorf("parse yaml: %w", err)
	}
	if err := expand(&root, lookup); err != nil {
		return Config{}, err
	}
	var f file
	if err := decodeStrict(&root, &f); err != nil {
		return Config{}, err
	}
	cfg := Config{Language: f.Language}
	if cfg.Language == "" {
		cfg.Language = DefaultLanguage
	}
	loc, err := loadLocation(f.Timezone)
	if err != nil {
		return Config{}, err
	}
	cfg.Timezone = loc
	for i, sec := range f.Sources {
		src, err := sec.source()
		if err != nil {
			return Config{}, fmt.Errorf("sources[%d]: %w", i, err)
		}
		cfg.Sources = append(cfg.Sources, src)
	}
	for i, sec := range f.Displays {
		d, err := sec.display(cfg.Language)
		if err != nil {
			return Config{}, fmt.Errorf("displays[%d]: %w", i, err)
		}
		cfg.Displays = append(cfg.Displays, d)
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Decode fills out with the section's kind-specific keys. Unknown keys and
// type mismatches are errors. An absent section decodes to the zero value.
func (o Options) Decode(out any) error {
	if o.node == nil {
		return nil
	}
	b, err := yaml.Marshal(o.node)
	if err != nil {
		return fmt.Errorf("options: encode: %w", err)
	}
	if err := decodeStrict(bytes.NewReader(b), out); err != nil {
		return fmt.Errorf("options: %w", err)
	}
	return nil
}

// UnmarshalYAML keeps the raw node of a list entry.
func (s *section) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: expected a mapping, got %s", n.Line, nodeKind(n))
	}
	s.node = *n
	return nil
}

func (s *section) source() (SourceSection, error) {
	head, rest := split(&s.node, sourceHead)
	var h sourceHeadFields
	if err := decodeStrict(head, &h); err != nil {
		return SourceSection{}, err
	}
	src := SourceSection{
		Kind:       h.Kind,
		Name:       h.Name,
		Interval:   h.Interval,
		StaleAfter: h.StaleAfter,
		Options:    Options{node: rest},
	}
	if src.Name == "" {
		src.Name = src.Kind
	}
	if src.Interval == 0 {
		src.Interval = DefaultInterval
	}
	if src.StaleAfter == 0 {
		src.StaleAfter = max(staleFactor*src.Interval, minStaleAfter)
	}
	return src, nil
}

func (s *section) display(language string) (DisplaySection, error) {
	head, rest := split(&s.node, displayHead)
	var h displayHeadFields
	if err := decodeStrict(head, &h); err != nil {
		return DisplaySection{}, err
	}
	d := DisplaySection{
		Kind:      h.Kind,
		Name:      h.Name,
		Scenes:    h.Scenes,
		Tick:      h.Tick,
		FullEvery: h.FullEvery,
		Language:  h.Language,
		Options:   Options{node: rest},
	}
	if d.Name == "" {
		d.Name = d.Kind
	}
	if len(d.Scenes) == 0 {
		d.Scenes = []string{"overview"}
	}
	if d.Tick == 0 {
		d.Tick = DefaultTick
	}
	if d.FullEvery == 0 {
		d.FullEvery = DefaultFullEvery
	}
	if d.Language == "" {
		d.Language = language
	}
	return d, nil
}

func (c Config) validate() error {
	if len(c.Displays) == 0 {
		return errors.New("at least one display is required")
	}
	seen := map[string]bool{}
	for i, s := range c.Sources {
		if err := validateHead("sources", i, s.Kind, s.Name, seen); err != nil {
			return err
		}
		if s.Interval < 0 {
			return fmt.Errorf("sources[%d] %s: interval must be positive", i, s.Name)
		}
	}
	seen = map[string]bool{}
	for i, d := range c.Displays {
		if err := validateHead("displays", i, d.Kind, d.Name, seen); err != nil {
			return err
		}
		if d.Tick < 0 || d.FullEvery < 0 {
			return fmt.Errorf("displays[%d] %s: tick and full_every must be positive", i, d.Name)
		}
		if slices.Contains(d.Scenes, "") {
			return fmt.Errorf("displays[%d] %s: empty scene name", i, d.Name)
		}
	}
	return nil
}

func validateHead(list string, i int, kind, name string, seen map[string]bool) error {
	if kind == "" {
		return fmt.Errorf("%s[%d]: kind is required", list, i)
	}
	if !nameRe.MatchString(name) {
		return fmt.Errorf("%s[%d]: name %q must match %s", list, i, name, nameRe)
	}
	if seen[name] {
		return fmt.Errorf("%s[%d]: duplicate name %q", list, i, name)
	}
	seen[name] = true
	return nil
}

// expand resolves ${NAME} placeholders in every string scalar of the tree.
func expand(n *yaml.Node, lookup Lookup) error {
	missing := map[string]bool{}
	var walk func(n *yaml.Node)
	walk = func(n *yaml.Node) {
		if n.Kind == yaml.ScalarNode && n.Tag == "!!str" && strings.Contains(n.Value, "${") {
			n.Value = placeholderRe.ReplaceAllStringFunc(n.Value, func(ph string) string {
				name := placeholderRe.FindStringSubmatch(ph)[1]
				v, ok := lookup(name)
				if !ok {
					missing[name] = true
					return ph
				}
				return v
			})
		}
		for _, c := range n.Content {
			walk(c)
		}
	}
	walk(n)
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("unresolved secrets: %s", strings.Join(slices.Sorted(maps.Keys(missing)), ", "))
}

// split partitions a mapping node into the keys named in head and the rest.
func split(n *yaml.Node, head []string) (headNode, rest *yaml.Node) {
	headNode = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	rest = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if slices.Contains(head, k.Value) {
			headNode.Content = append(headNode.Content, k, v)
		} else {
			rest.Content = append(rest.Content, k, v)
		}
	}
	return headNode, rest
}

// decodeStrict decodes a node or a reader into out, rejecting unknown keys.
func decodeStrict(src, out any) error {
	var dec *yaml.Decoder
	switch s := src.(type) {
	case *yaml.Node:
		b, err := yaml.Marshal(s)
		if err != nil {
			return fmt.Errorf("encode: %w", err)
		}
		dec = yaml.NewDecoder(bytes.NewReader(b))
	case *bytes.Reader:
		dec = yaml.NewDecoder(s)
	default:
		return fmt.Errorf("decodeStrict: unsupported source %T", src)
	}
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	return nil
}

func loadLocation(name string) (*time.Location, error) {
	if name == "" {
		return time.Local, nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("timezone %q: %w", name, err)
	}
	return loc, nil
}

func nodeKind(n *yaml.Node) string {
	switch n.Kind {
	case yaml.ScalarNode:
		return "a scalar"
	case yaml.SequenceNode:
		return "a sequence"
	case yaml.MappingNode:
		return "a mapping"
	default:
		return "an unexpected node"
	}
}
