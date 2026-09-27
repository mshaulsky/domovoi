// Package modules is the glue between the domain packages and the
// framework: one file per module, each registering a domain package's kind
// or singleton in the container and decoding its YAML section into the
// package's plain Config. Domain packages never import this package or the
// container.
package modules
