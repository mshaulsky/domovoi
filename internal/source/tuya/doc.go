// Package tuya is the source for devices in a Tuya cloud project — the
// climate sensors behind the Bluetooth gateway today. Every poll reads the
// batched status; every ListEvery-th poll also lists the devices (names,
// online flags, the device list itself) — the listing is a request of its
// own and online flags of battery sensors do not change by the minute. Raw
// integers are scaled through each product's specification, fetched once
// and cached. Data points that map to no metric are dropped. Readings are
// stamped with the wall clock; under testing/synctest it is virtual.
package tuya
