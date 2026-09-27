package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestSecretReveal(t *testing.T) {
	if got := Secret("hunter2").Reveal(); got != "hunter2" {
		t.Errorf("Reveal() = %q", got)
	}
}

func TestSecretString(t *testing.T) {
	type holder struct {
		Key Secret
	}
	tests := []struct {
		name string
		got  string
	}{
		{name: "String", got: Secret("hunter2").String()},
		{name: "%v", got: fmt.Sprintf("key=%v", Secret("hunter2"))},
		{name: "%s", got: fmt.Sprintf("key=%s", Secret("hunter2"))},
		{name: "%+v of a struct", got: fmt.Sprintf("%+v", holder{Key: "hunter2"})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if strings.Contains(tt.got, "hunter2") || !strings.Contains(tt.got, "***") {
				t.Errorf("rendered as %q, want redacted", tt.got)
			}
		})
	}
}

func TestSecretLogValue(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	log.Info("login", "key", Secret("hunter2"))
	if strings.Contains(buf.String(), "hunter2") || !strings.Contains(buf.String(), "key=***") {
		t.Errorf("log line %q leaks the secret", buf.String())
	}
}

func TestSecretMarshalYAML(t *testing.T) {
	out, err := yaml.Marshal(struct {
		Key Secret `yaml:"key"`
	}{Key: "hunter2"})
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "key: '***'\n" {
		t.Errorf("yaml = %q", out)
	}
}

func TestSecretMarshalText(t *testing.T) {
	out, err := json.Marshal(map[string]Secret{"key": "hunter2"})
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"key":"***"}` {
		t.Errorf("json = %s", out)
	}
}
