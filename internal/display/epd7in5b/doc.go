// Package epd7in5b is the display backend for the Waveshare 7.5" (B) V2
// three-colour e-ink panel, driven through the waveshare-epd7in5b-v2 driver.
//
// Every Show is one cycle: power-up for the frame's mode (Init, InitFast or
// InitPartial), the transfer, then Sleep — the panel never stays powered
// between frames. Sleep runs even when the refresh failed or was cancelled,
// on a budget of its own.
//
// A partial update is monochrome: the controller cannot draw or erase red
// inside the window, so the refresh policy keeps red out of partial windows
// and sends a colour refresh instead.
//
// NopPanel is a panel that does nothing: -check builds the display against it so
// the configuration is validated end to end without opening SPI or GPIO.
package epd7in5b
