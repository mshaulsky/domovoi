// Package metrics is the one implementation of every package's Metrics
// interface. Packages count in their own terms (IncPollSuccess, IncRender);
// this registry turns those into a handful of Prometheus families on top of
// the official client library and serves them through Handler. Nothing
// outside this package imports client_golang.
package metrics
