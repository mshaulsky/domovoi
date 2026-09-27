// Package source defines how data enters the application. A Source is polled
// and answers with a Batch of devices and readings; a Watcher pushes into
// the same Sink on its own schedule. The Poller turns a Source into Sink
// calls, with backoff on failure and health reports so the core can show
// that a source is down instead of pretending its devices went offline.
package source
