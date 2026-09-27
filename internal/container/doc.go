// Package container is the dependency injection of the application: lazy
// singletons that build on first use, and registries of constructors keyed
// by the kind names the configuration uses. No reflection, no framework —
// a generic and a map. The Container struct is the manifest of singletons:
// adding one edits it, deliberately, so the wiring stays visible in one
// place.
package container
