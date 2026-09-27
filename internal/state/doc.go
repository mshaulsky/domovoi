// Package state holds what is true now: the latest reading of every metric
// of every device, the devices themselves, and the health of every source.
// It is fed by the core and read through immutable snapshots. Applying
// readings reports changes, which the classifier turns into events using
// the metric catalogue.
package state
