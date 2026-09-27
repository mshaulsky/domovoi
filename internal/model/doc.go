// Package model is the canonical, vendor-neutral view of the home. Every
// source maps its native data into these types; scenes, alert rules and the
// history store consume nothing else.
//
// A Reading is a level ("the bedroom is at 22.4 °C"), an Event is a
// transition ("the door was unlocked"). The metric catalogue ties the two
// together: it says what kind of value each metric carries, its unit, and
// which events a change of a boolean metric produces.
package model
