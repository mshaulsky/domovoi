// Package aqara is the source for devices in an Aqara Home account, read
// through the Aqara MCP server — the door lock, the leak sensors, the
// washing-machine outlet and the button today. One status call per poll
// returns every device with its name, type, room and state, so each batch
// carries the device list. The status is a flat key/value text: keys that
// map to no metric are dropped. The service carries no timestamps, so
// readings are stamped with the poll's wall clock; under testing/synctest
// it is virtual.
package aqara
