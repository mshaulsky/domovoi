// Package store is the unit of work over the repositories: one transaction
// per poll batch that upserts devices, writes readings and journals events
// together, the restore query at start, pruning, and the history queries
// scenes read. It is the only place that opens transactions; the core sees
// it through its own small interfaces and never touches SQL.
package store
