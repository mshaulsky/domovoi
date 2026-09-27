package i18n

import (
	"embed"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
	"go.yaml.in/yaml/v3"
	"golang.org/x/text/language"
)

// Bundle localises into one language. A key the language lacks falls back
// to English; a key no catalogue has renders as the key itself, so a gap is
// visible on screen rather than fatal.
type Bundle struct {
	lang    string
	loc     *goi18n.Localizer
	keys    []string // message IDs of this language, sorted
	decimal string   // decimal separator
}

// Args carries the values a message template refers to by name:
// T("scene.updated", Args{"Time": "12:05"}) fills "updated {{.Time}}".
type Args map[string]any

// Conventions that are not worth a catalogue entry.
const (
	timeLayout = "15:04"
	yamlFormat = "yaml"
)

//go:embed locales/*.yaml
var locales embed.FS

// decimals is the decimal separator per language; "." when unlisted.
var decimals = map[string]string{
	"ru": ",",
}

// Load returns the bundle of a language, "ru" or "en".
func Load(lang string) (*Bundle, error) {
	langs := Languages()
	if !slices.Contains(langs, lang) {
		return nil, fmt.Errorf("i18n: language %q: not embedded (available: %s)", lang, strings.Join(langs, ", "))
	}
	bundle := goi18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc(yamlFormat, yaml.Unmarshal)
	b := &Bundle{lang: lang, decimal: "."}
	if d, ok := decimals[lang]; ok {
		b.decimal = d
	}
	// Every language goes in so the requested one can fall back to English.
	for _, l := range langs {
		file := l + "." + yamlFormat
		data, err := locales.ReadFile(path.Join("locales", file))
		if err != nil {
			return nil, fmt.Errorf("i18n: read %s: %w", file, err)
		}
		mf, err := bundle.ParseMessageFileBytes(data, file)
		if err != nil {
			return nil, fmt.Errorf("i18n: parse %s: %w", file, err)
		}
		if l != lang {
			continue
		}
		for _, m := range mf.Messages {
			b.keys = append(b.keys, m.ID)
		}
	}
	slices.Sort(b.keys)
	b.loc = goi18n.NewLocalizer(bundle, lang)
	return b, nil
}

// Languages lists the embedded languages, sorted.
func Languages() []string {
	entries, err := fs.ReadDir(locales, "locales")
	if err != nil {
		return nil
	}
	langs := make([]string, 0, len(entries))
	for _, e := range entries {
		langs = append(langs, strings.TrimSuffix(e.Name(), "."+yamlFormat))
	}
	slices.Sort(langs)
	return langs
}

// Lang returns the bundle's language code.
func (b *Bundle) Lang() string {
	return b.lang
}

// T returns the message for key, its placeholders filled from args when
// there are any.
func (b *Bundle) T(key string, args ...Args) string {
	return b.localize(&goi18n.LocalizeConfig{MessageID: key, TemplateData: merge(args)}, key)
}

// N returns the plural form of key for n. The template sees n as
// {{.PluralCount}} next to args.
func (b *Bundle) N(key string, n int, args ...Args) string {
	data := merge(args)
	if data == nil {
		data = map[string]any{}
	}
	data["PluralCount"] = n
	return b.localize(&goi18n.LocalizeConfig{MessageID: key, PluralCount: n, TemplateData: data}, key)
}

// Has reports whether this language's catalogue defines key.
func (b *Bundle) Has(key string) bool {
	_, ok := slices.BinarySearch(b.keys, key)
	return ok
}

// Keys lists every key of this language's catalogue, sorted, for tests and
// tooling.
func (b *Bundle) Keys() []string {
	return slices.Clone(b.keys)
}

// Number formats v with the given decimals and the language's separator.
// A negative zero is normalised so "-0,0 °C" never shows.
func (b *Bundle) Number(v float64, decimals int) string {
	s := strconv.FormatFloat(v, 'f', decimals, 64)
	if strings.Trim(s, "-0.") == "" {
		s = strings.TrimPrefix(s, "-")
	}
	return strings.Replace(s, ".", b.decimal, 1)
}

// Time formats the wall-clock part of t as HH:MM.
func (b *Bundle) Time(t time.Time) string {
	return t.Format(timeLayout)
}

// Date formats t as a short calendar line: "Sun 13 Sep" or "вс 13 сен".
func (b *Bundle) Date(t time.Time) string {
	weekday := b.T("date.weekday." + strconv.Itoa(int(t.Weekday())))
	month := b.T("date.month." + strconv.Itoa(int(t.Month())))
	return fmt.Sprintf("%s %d %s", weekday, t.Day(), month)
}

// localize runs one lookup; go-i18n returns the English text together with
// a not-found error when the language lacks the key, so only an empty
// result means the key is unknown everywhere.
func (b *Bundle) localize(cfg *goi18n.LocalizeConfig, key string) string {
	s, _ := b.loc.Localize(cfg)
	if s == "" {
		return key
	}
	return s
}

// merge folds the optional Args into one template data map; nil when none.
func merge(args []Args) map[string]any {
	if len(args) == 0 {
		return nil
	}
	data := make(map[string]any)
	for _, a := range args {
		maps.Copy(data, a)
	}
	return data
}
