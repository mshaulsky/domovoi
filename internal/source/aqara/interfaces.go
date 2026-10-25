package aqara

import (
	"context"
	"time"

	"github.com/mshaulsky/aqaramcp"
)

//go:generate go tool mockgen -source=interfaces.go -destination=mocks_test.go -package=aqara

// Client is the part of the Aqara MCP client the source uses.
type Client interface {
	Statuses(ctx context.Context, filter aqaramcp.StatusFilter) ([]aqaramcp.DeviceStatus, error)
	Homes(ctx context.Context) ([]aqaramcp.Home, error)
	AutomationRuns(
		ctx context.Context,
		from, to time.Time,
		filter aqaramcp.AutomationFilter,
	) ([]aqaramcp.AutomationRun, error)
}

// Metrics counts upstream requests, for quota tracking.
type Metrics interface {
	IncRequest(source string)
}
