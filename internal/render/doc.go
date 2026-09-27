// Package render drives one display: on every tick it takes a snapshot of
// the state, builds the scene's View, renders the current page and decides
// how the frame reaches the glass. Rendering is tick-driven, not
// change-driven — that is what keeps the refresh budget: readings arriving
// between ticks wait, and only a Wake (an alert transition, a command)
// renders out of band.
package render
