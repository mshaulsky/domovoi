package tuya

import (
	"context"

	"github.com/mshaulsky/tuyacloud"
)

//go:generate go tool mockgen -source=interfaces.go -destination=mocks_test.go -package=tuya

// Client is the part of the Tuya cloud client the source uses.
type Client interface {
	Devices(ctx context.Context) ([]tuyacloud.Device, error)
	DevicesStatus(ctx context.Context, deviceIDs ...string) (map[string]tuyacloud.Status, error)
	Specification(ctx context.Context, deviceID string) (tuyacloud.Specification, error)
}

// Metrics counts upstream requests, for quota tracking.
type Metrics interface {
	IncRequest(source string)
}
