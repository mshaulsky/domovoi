package weather

import "net/http"

//go:generate go tool mockgen -source=interfaces.go -destination=mocks_test.go -package=weather

// Doer is the part of an HTTP client the source uses.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Metrics counts upstream requests, for quota tracking.
type Metrics interface {
	IncRequest(source string)
}
