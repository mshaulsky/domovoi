// Package scene turns a View — the render snapshot — into a frame for a
// Surface. Widgets draw with semantic colours (paper, ink, accent) that a
// Theme maps onto the display's palette, text and icons are rasterised
// through a coverage mask and thresholded, so every edge is crisp on a
// 1-bit panel, and layout is proportional to the surface, so one scene fits
// 800×480 and a small mono panel alike. A scene is a pure function of its
// inputs: golden images are its tests.
package scene
