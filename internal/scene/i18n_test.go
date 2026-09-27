package scene

import (
	"testing"

	"github.com/mshaulsky/domovoi/internal/i18n"
	"github.com/mshaulsky/domovoi/internal/model"
)

// TestCatalogueCoversConstants guards the keys widgets build from constants
// ("event."+kind, "weather."+condition, "alert."+severity): every one must
// have a message in every language, or a raw key ends up on the glass.
func TestCatalogueCoversConstants(t *testing.T) {
	var keys []string
	for _, k := range model.EventKinds() {
		keys = append(keys, "event."+string(k))
	}
	for _, c := range model.WeatherConditions() {
		keys = append(keys, "weather."+string(c))
	}
	for _, s := range []string{SeverityUrgent, SeverityWarning, SeverityInfo} {
		keys = append(keys, "alert."+s)
	}
	for _, lang := range i18n.Languages() {
		b, err := i18n.Load(lang)
		if err != nil {
			t.Fatal(err)
		}
		for _, key := range keys {
			if !b.Has(key) {
				t.Errorf("%s: no message for %s", lang, key)
			}
		}
	}
}
