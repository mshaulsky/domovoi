// Package i18n renders every human-readable string of the application —
// on screen, in Telegram, in the back office — from embedded message
// catalogues, one YAML file per language, through go-i18n. Keys are
// namespaced by surface (scene.updated, event.unlocked, weather.rain);
// messages name their placeholders ("updated {{.Time}}") so a translation
// may reorder them, and plural entries carry the CLDR forms the language
// needs. Numbers and dates follow the language's conventions too, so a
// Russian frame reads "22,4 °C" and an English one "22.4 °C".
package i18n
