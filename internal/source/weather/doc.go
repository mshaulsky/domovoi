// Package weather is the virtual source for the outside: current
// conditions, today's forecast and sun times for one location from
// Open-Meteo (free, no key). It owns a single device, <name>:home, of kind
// Virtual, and proves the path every non-vendor source will take: nothing
// in the core knows the readings come from a forecast rather than a sensor.
// One request per poll; readings are stamped with the model's own time for
// the current conditions.
package weather
