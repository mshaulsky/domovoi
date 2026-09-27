package scene

import "testing"

func TestLoadFonts(t *testing.T) {
	f, err := LoadFonts()
	if err != nil {
		t.Fatal(err)
	}
	if f.text == nil || f.bold == nil || f.icons == nil {
		t.Error("a font is missing")
	}
}

func TestDefaultFonts(t *testing.T) {
	a, err := DefaultFonts()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := DefaultFonts()
	if a != b {
		t.Error("DefaultFonts parsed twice")
	}
}

func TestFontsFaces(t *testing.T) {
	f, _ := DefaultFonts()
	a, b := f.Faces(), f.Faces()
	if a == b || a.Text(20) == b.Text(20) {
		t.Error("each Faces must own its faces: a face is not safe to share between renders")
	}
}

func TestFacesText(t *testing.T) {
	f, _ := DefaultFonts()
	faces := f.Faces()
	if faces.Text(20) != faces.Text(20) {
		t.Error("faces are not cached within one render")
	}
	if faces.Text(20) == faces.Text(21) {
		t.Error("different sizes share a face")
	}
	if faces.Text(0) == nil {
		t.Error("size 0 should fall back to the smallest face")
	}
}

func TestFacesBold(t *testing.T) {
	f, _ := DefaultFonts()
	faces := f.Faces()
	if faces.Bold(20) == faces.Text(20) {
		t.Error("bold and regular share a face")
	}
}

func TestFacesIcon(t *testing.T) {
	f, _ := DefaultFonts()
	faces := f.Faces()
	if faces.Icon(24) == nil || faces.Icon(24) == faces.Text(24) {
		t.Error("icon face wrong")
	}
}
