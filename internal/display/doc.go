// Package display is the contract between the render coordinator and a
// physical or virtual display. A display declares its Surface (size,
// palette, whether it can update a window) and shows Frames already
// rendered in its palette; deciding how to refresh is the coordinator's
// job, drawing is the scene's.
package display
