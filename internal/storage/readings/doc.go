// Package readings is the repository of the series and readings tables:
// the history of every metric of every device, one row per change plus a
// heartbeat, keyed by an integer series ID instead of the long device ID.
// It writes what it is given and answers the queries scenes need; the
// write policy (what to keep, when to prune) belongs to the core.
package readings
