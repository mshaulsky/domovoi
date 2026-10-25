package modules

import (
	"strings"
	"testing"
)

func TestAqaraRegister(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "key",
			yaml: "sources:\n  - kind: aqara\n    api_key: ${KEY}\n" + base,
		},
		{
			name: "own endpoint",
			yaml: "sources:\n  - kind: aqara\n    api_key: ${KEY}\n    endpoint: http://localhost:9/mcp\n" + base,
		},
		{
			name: "history zone",
			yaml: "sources:\n  - kind: aqara\n    api_key: ${KEY}\n    history_zone: Asia/Shanghai\n" + base,
		},
		{
			name:    "an unknown history zone is refused",
			yaml:    "sources:\n  - kind: aqara\n    api_key: ${KEY}\n    history_zone: Mars/Olympus\n" + base,
			wantErr: "history_zone",
		},
		{
			name: "account",
			yaml: "sources:\n  - kind: aqara\n    username: home@example.com\n" +
				"    password_md5: 3cb4e732631f47e6eb961f34554b7cde\n    region: ru\n" + base,
		},
		{
			name: "the password itself is refused",
			yaml: "sources:\n  - kind: aqara\n    username: home@example.com\n    password_md5: ${KEY}\n" +
				"    region: RU\n" + base,
			wantErr: "password MD5 must be 32 hex digits",
		},
		{
			name: "key and account",
			yaml: "sources:\n  - kind: aqara\n    api_key: ${KEY}\n    username: home@example.com\n" +
				"    password_md5: 3cb4e732631f47e6eb961f34554b7cde\n    region: RU\n" + base,
		},
		{
			name:    "key or account required",
			yaml:    "sources:\n  - kind: aqara\n" + base,
			wantErr: "api_key or an account (username, password_md5, region) is required",
		},
		{
			name:    "password without username",
			yaml:    "sources:\n  - kind: aqara\n    password_md5: 3cb4e732631f47e6eb961f34554b7cde\n" + base,
			wantErr: "username is required with password_md5",
		},
		{
			name:    "account without password",
			yaml:    "sources:\n  - kind: aqara\n    username: home@example.com\n    region: RU\n" + base,
			wantErr: "password_md5 is required with username",
		},
		{
			name: "account without region",
			yaml: "sources:\n  - kind: aqara\n    username: home@example.com\n" +
				"    password_md5: 3cb4e732631f47e6eb961f34554b7cde\n" + base,
			wantErr: "region is required with username",
		},
		{
			name: "unknown region",
			yaml: "sources:\n  - kind: aqara\n    username: home@example.com\n" +
				"    password_md5: 3cb4e732631f47e6eb961f34554b7cde\n    region: mars\n" + base,
			wantErr: `region "mars" is not one of EU, RU, US, CN, KR, SG`,
		},
		{
			name: "automations",
			yaml: "sources:\n  - kind: aqara\n    api_key: ${KEY}\n    automations:\n" +
				"      - {match: КОГДА Разблокировать, device: aqara:doorlock-3b7f29e4, event: unlocked}\n" +
				"      - {match: КОГДА Заблокировать, device: aqara:doorlock-3b7f29e4, event: locked}\n" + base,
		},
		{
			name: "an automation without a match",
			yaml: "sources:\n  - kind: aqara\n    api_key: ${KEY}\n    automations:\n" +
				"      - {match: \" \", device: aqara:doorlock-3b7f29e4, event: unlocked}\n" + base,
			wantErr: "automations[0]: match is required",
		},
		{
			name: "an automation of another source's device",
			yaml: "sources:\n  - kind: aqara\n    api_key: ${KEY}\n    automations:\n" +
				"      - {match: КОГДА Разблокировать, device: tuya:lock, event: unlocked}\n" + base,
			wantErr: `automations[0]: device "tuya:lock": want aqara:<id>`,
		},
		{
			name: "an automation of no device event",
			yaml: "sources:\n  - kind: aqara\n    api_key: ${KEY}\n    automations:\n" +
				"      - {match: КОГДА Разблокировать, device: aqara:doorlock-3b7f29e4, event: alert_raised}\n" + base,
			wantErr: `automations[0]: event "alert_raised" is not a device event`,
		},
		{
			name:    "unknown option",
			yaml:    "sources:\n  - kind: aqara\n    apikey: ${KEY}\n" + base,
			wantErr: "field apikey not found",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newContainer()
			if err := (Aqara{}).Register(c); err != nil {
				t.Fatal(err)
			}
			ctor, err := c.Sources.Get("aqara")
			if err != nil {
				t.Fatal(err)
			}
			src, err := ctor(parse(t, tt.yaml).Sources[0])
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if src.Name() != "aqara" {
				t.Errorf("source name = %q", src.Name())
			}
		})
	}
	t.Run("lifecycle", func(t *testing.T) {
		m := Aqara{}
		if m.Name() != "aqara" || m.Start(t.Context()) != nil || m.Stop(t.Context()) != nil {
			t.Error("passive lifecycle misbehaves")
		}
	})
}
