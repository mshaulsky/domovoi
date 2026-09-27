// Package icon draws pictograms. Icons are glyphs of an embedded subset of
// the Material Design Icons font (Pictogrammers, Apache 2.0), rasterised by
// the same opentype path as text, so they scale to any display and take the
// theme's colours like text does.
//
// The subset holds only the glyphs named in names.go. Regenerate it with
// `make icons` after adding a constant: the target lists the code points
// (cmd/codepoints) and runs tools/subset-icons.mjs over the upstream font.
// A test guards against a stale subset by resolving every constant to a real
// glyph.
package icon
