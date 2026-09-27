package config

import "log/slog"

// Secret is a string that refuses to print. It reaches logs, %v, YAML dumps
// and templates as "***"; only Reveal returns the value, and only the
// assembly glue should call it, right before handing the value to a client.
type Secret string

// redacted is what every rendering of a Secret shows.
const redacted = "***"

// Reveal returns the secret itself.
func (s Secret) Reveal() string {
	return string(s)
}

// String implements fmt.Stringer with the redacted form.
func (s Secret) String() string {
	return redacted
}

// LogValue implements slog.LogValuer with the redacted form.
func (s Secret) LogValue() slog.Value {
	return slog.StringValue(redacted)
}

// MarshalYAML implements yaml.Marshaler with the redacted form, so a config
// dumped back to YAML (the back office, a debug page) never carries the value.
func (s Secret) MarshalYAML() (any, error) {
	return redacted, nil
}

// MarshalText implements encoding.TextMarshaler with the redacted form, which
// covers JSON and text templates too.
func (s Secret) MarshalText() ([]byte, error) {
	return []byte(redacted), nil
}
