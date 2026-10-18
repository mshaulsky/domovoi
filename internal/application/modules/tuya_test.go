package modules

import (
	"strings"
	"testing"
)

func TestTuyaRegister(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "valid",
			yaml: "sources:\n  - kind: tuya\n    access_id: ${ID}\n    access_key: ${KEY}\n    region: eu\n" + base,
		},
		{
			name: "default region",
			yaml: "sources:\n  - kind: tuya\n    access_id: ${ID}\n    access_key: ${KEY}\n" + base,
		},
		{
			name:    "credentials required",
			yaml:    "sources:\n  - kind: tuya\n    region: eu\n" + base,
			wantErr: "access_id and access_key are required",
		},
		{
			name: "unknown region",
			yaml: "sources:\n  - kind: tuya\n    access_id: ${ID}\n    access_key: ${KEY}\n" +
				"    region: mars\n" + base,
			wantErr: `unknown region "mars"`,
		},
		{
			name:    "unknown option",
			yaml:    "sources:\n  - kind: tuya\n    acess_id: ${ID}\n    access_key: ${KEY}\n" + base,
			wantErr: "field acess_id not found",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newContainer()
			if err := (Tuya{}).Register(c); err != nil {
				t.Fatal(err)
			}
			ctor, err := c.Sources.Get("tuya")
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
			if src.Name() != "tuya" {
				t.Errorf("source name = %q", src.Name())
			}
		})
	}
	t.Run("lifecycle", func(t *testing.T) {
		m := Tuya{}
		if m.Name() != "tuya" || m.Start(t.Context()) != nil || m.Stop(t.Context()) != nil {
			t.Error("passive lifecycle misbehaves")
		}
	})
}
