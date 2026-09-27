// Package core is the orchestrator: it owns the State, is the Sink every
// poller and watcher deliver into, turns changes into journal events, runs
// the pollers and the render coordinators, and implements the Commands the
// button, the back office and Telegram call. It knows every domain package
// and nothing about configuration or the container.
package core
