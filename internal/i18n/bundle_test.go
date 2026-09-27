package i18n

import (
	"path"
	"slices"
	"strings"
	"testing"
	"time"

	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
	"go.yaml.in/yaml/v3"
)

func mustLoad(t *testing.T, lang string) *Bundle {
	t.Helper()
	b, err := Load(lang)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// messages parses one embedded catalogue the way Load does and indexes the
// messages by ID, so a test can inspect plural forms.
func messages(t *testing.T, lang string) map[string]*goi18n.Message {
	t.Helper()
	file := lang + ".yaml"
	data, err := locales.ReadFile(path.Join("locales", file))
	if err != nil {
		t.Fatal(err)
	}
	mf, err := goi18n.ParseMessageFileBytes(data, file, map[string]goi18n.UnmarshalFunc{"yaml": yaml.Unmarshal})
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]*goi18n.Message, len(mf.Messages))
	for _, m := range mf.Messages {
		byID[m.ID] = m
	}
	return byID
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		lang    string
		wantErr string
	}{
		{name: "english", lang: "en"},
		{name: "russian", lang: "ru"},
		{name: "unknown", lang: "xx", wantErr: `language "xx"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := Load(tt.lang)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if b.Lang() != tt.lang {
				t.Errorf("Lang() = %q, want %q", b.Lang(), tt.lang)
			}
		})
	}
}

func TestLanguages(t *testing.T) {
	if got := Languages(); !slices.Equal(got, []string{"en", "ru"}) {
		t.Errorf("Languages() = %v, want [en ru]", got)
	}
}

// TestCataloguesAgree is the guard against a key added to one language only,
// and against a plural entry lacking a form its language needs.
func TestCataloguesAgree(t *testing.T) {
	en, ru := mustLoad(t, "en"), mustLoad(t, "ru")
	if !slices.Equal(en.Keys(), ru.Keys()) {
		t.Errorf("key sets differ:\nen: %v\nru: %v", en.Keys(), ru.Keys())
	}
	for id, m := range messages(t, "ru") {
		if m.One != "" && (m.Few == "" || m.Many == "") {
			t.Errorf("ru plural %s lacks few or many", id)
		}
	}
	for id, m := range messages(t, "en") {
		if m.One != "" && m.Other == "" {
			t.Errorf("en plural %s lacks other", id)
		}
	}
}

func TestBundleT(t *testing.T) {
	tests := []struct {
		name string
		lang string
		key  string
		args []Args
		want string
	}{
		{name: "plain", lang: "en", key: "scene.outside", want: "Outside"},
		{name: "placeholder", lang: "en", key: "scene.updated", args: []Args{{"Time": "12:05"}}, want: "updated 12:05"},
		{name: "russian", lang: "ru", key: "scene.updated", args: []Args{{"Time": "12:05"}}, want: "обновлено 12:05"},
		{name: "percent sign", lang: "en", key: "scene.precipitation", args: []Args{{"Percent": "40"}}, want: "40 %"},
		{name: "two placeholders", lang: "ru", key: "scene.min_max", args: []Args{{"Min": "18,2"}, {"Max": "24,0"}}, want: "18,2 / 24,0"},
		{name: "subject", lang: "en", key: "event.unlocked", args: []Args{{"Subject": "Door"}}, want: "Door unlocked"},
		{name: "unknown key returns key", lang: "en", key: "scene.nope", want: "scene.nope"},
		{name: "date key", lang: "ru", key: "date.month.5", want: "мая"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mustLoad(t, tt.lang).T(tt.key, tt.args...); got != tt.want {
				t.Errorf("T() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBundleN(t *testing.T) {
	tests := []struct {
		name string
		lang string
		key  string
		n    int
		want string
	}{
		{name: "en one", lang: "en", key: "time.hours_ago", n: 1, want: "1 hour ago"},
		{name: "en other", lang: "en", key: "time.hours_ago", n: 2, want: "2 hours ago"},
		{name: "en zero", lang: "en", key: "time.minutes_ago", n: 0, want: "0 minutes ago"},
		{name: "ru one", lang: "ru", key: "time.hours_ago", n: 1, want: "1 час назад"},
		{name: "ru few", lang: "ru", key: "time.hours_ago", n: 3, want: "3 часа назад"},
		{name: "ru many", lang: "ru", key: "time.hours_ago", n: 5, want: "5 часов назад"},
		{name: "ru eleven is many", lang: "ru", key: "time.hours_ago", n: 11, want: "11 часов назад"},
		{name: "ru twenty-one is one", lang: "ru", key: "time.hours_ago", n: 21, want: "21 час назад"},
		{name: "ru twenty-two is few", lang: "ru", key: "time.days_ago", n: 22, want: "22 дня назад"},
		{name: "ru hundred-twelve is many", lang: "ru", key: "time.days_ago", n: 112, want: "112 дней назад"},
		{name: "unknown key", lang: "en", key: "time.weeks_ago", n: 2, want: "time.weeks_ago"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mustLoad(t, tt.lang).N(tt.key, tt.n); got != tt.want {
				t.Errorf("N() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBundleHas(t *testing.T) {
	b := mustLoad(t, "en")
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{name: "message", key: "scene.outside", want: true},
		{name: "plural", key: "time.hours_ago", want: true},
		{name: "nested", key: "date.weekday.0", want: true},
		{name: "namespace is not a key", key: "scene", want: false},
		{name: "unknown", key: "scene.nope", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := b.Has(tt.key); got != tt.want {
				t.Errorf("Has(%q) = %t, want %t", tt.key, got, tt.want)
			}
		})
	}
}

func TestBundleKeys(t *testing.T) {
	keys := mustLoad(t, "en").Keys()
	if !slices.IsSorted(keys) {
		t.Error("Keys() is not sorted")
	}
	if !slices.Contains(keys, "scene.outside") || !slices.Contains(keys, "time.hours_ago") {
		t.Errorf("Keys() lacks expected keys: %v", keys)
	}
}

func TestBundleNumber(t *testing.T) {
	tests := []struct {
		name     string
		lang     string
		v        float64
		decimals int
		want     string
	}{
		{name: "en one decimal", lang: "en", v: 22.44, decimals: 1, want: "22.4"},
		{name: "ru one decimal", lang: "ru", v: 22.44, decimals: 1, want: "22,4"},
		{name: "integer", lang: "ru", v: 48, decimals: 0, want: "48"},
		{name: "negative", lang: "ru", v: -3.5, decimals: 1, want: "-3,5"},
		{name: "negative zero normalised", lang: "en", v: -0.04, decimals: 1, want: "0.0"},
		{name: "rounds half away", lang: "en", v: 0.05, decimals: 1, want: "0.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mustLoad(t, tt.lang).Number(tt.v, tt.decimals); got != tt.want {
				t.Errorf("Number() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBundleTime(t *testing.T) {
	at := time.Date(2026, 9, 13, 7, 5, 9, 0, time.UTC)
	if got := mustLoad(t, "en").Time(at); got != "07:05" {
		t.Errorf("Time() = %q, want 07:05", got)
	}
}

func TestBundleDate(t *testing.T) {
	at := time.Date(2026, 9, 13, 7, 5, 9, 0, time.UTC) // a Sunday
	tests := []struct {
		name string
		lang string
		want string
	}{
		{name: "english", lang: "en", want: "Sun 13 Sep"},
		{name: "russian", lang: "ru", want: "вс 13 сен"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mustLoad(t, tt.lang).Date(at); got != tt.want {
				t.Errorf("Date() = %q, want %q", got, tt.want)
			}
		})
	}
}
