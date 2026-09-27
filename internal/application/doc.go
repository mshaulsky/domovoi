// Package application assembles the binary: it loads the configuration,
// builds the container, lets every module (container.Module) register what
// it provides, creates the configured source and display instances, and
// runs the modules' lifecycle — Start top-down, Stop bottom-up. cmd/domovoi
// calls Run and nothing else.
package application
