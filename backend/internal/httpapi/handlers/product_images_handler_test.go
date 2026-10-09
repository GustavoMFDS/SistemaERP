package handlers

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

func catalogTestJPEG(t *testing.T, w, h int) string {
	t.Helper()
	var out bytes.Buffer
	pic := image.NewRGBA(image.Rect(0, 0, w, h))
	if err := jpeg.Encode(&out, pic, &jpeg.Options{Quality: 70}); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(out.Bytes())
}

func TestCatalogJPEGAcceptsBoundedFile(t *testing.T) {
	data := catalogTestJPEG(t, 32, 32)
	got, err := catalogJPEG(data, maxCatalogPhotoBytes, 1600)
	if err != nil || len(got) == 0 {
		t.Fatalf("valid image rejected: length=%d err=%v", len(got), err)
	}
}

func TestCatalogJPEGRejectsInvalidData(t *testing.T) {
	for name, encoded := range map[string]string{
		"empty":          "",
		"invalid base64": "!!!",
		"html":           base64.StdEncoding.EncodeToString([]byte("<script>alert(1)</script>")),
		"wrong file":     base64.StdEncoding.EncodeToString([]byte("GIF89a")),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := catalogJPEG(encoded, maxCatalogPhotoBytes, 1600); err == nil {
				t.Fatal("invalid image accepted")
			}
		})
	}
}

func TestCatalogJPEGRejectsOversizedPixelDimensions(t *testing.T) {
	data := catalogTestJPEG(t, 1601, 2)
	if _, err := catalogJPEG(data, maxCatalogPhotoBytes, 1600); err == nil {
		t.Fatal("overlarge dimensions accepted")
	}
}

func TestCatalogJPEGRejectsOversizedEncodedInput(t *testing.T) {
	data := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0xff}, maxCatalogPhotoBytes+1))
	if _, err := catalogJPEG(data, maxCatalogPhotoBytes, 1600); err == nil {
		t.Fatal("overlarge base64 data accepted")
	}
}

func TestCatalogJPEGRemovesAppendedNonImagePayload(t *testing.T) {
	raw, err := base64.StdEncoding.DecodeString(catalogTestJPEG(t, 32, 32))
	if err != nil {
		t.Fatal(err)
	}
	suffix := []byte("<svg onload=alert(1)>")
	raw = append(raw, suffix...)
	clean, err := catalogJPEG(base64.StdEncoding.EncodeToString(raw), maxCatalogPhotoBytes, 1600)
	// A strict decoder may reject trailing bytes entirely. Either rejection
	// or safe re-encoding is acceptable; retaining the suffix is not.
	if err == nil && bytes.Contains(clean, suffix) {
		t.Fatal("untrusted suffix survived JPEG recompression")
	}
}

func TestCatalogThumbnailGeneratedFromOriginalPixels(t *testing.T) {
	pic := image.NewRGBA(image.Rect(0, 0, 600, 300))
	for y := 0; y < 300; y++ {
		for x := 0; x < 600; x++ {
			pic.Set(x, y, color.RGBA{R: 230, G: 30, B: 15, A: 255})
		}
	}
	var src bytes.Buffer
	if err := jpeg.Encode(&src, pic, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
	normalized, err := catalogJPEG(base64.StdEncoding.EncodeToString(src.Bytes()), maxCatalogPhotoBytes, 1600)
	if err != nil {
		t.Fatalf("normalizing image: %v", err)
	}
	thumb, err := catalogThumbnailJPEG(normalized)
	if err != nil || len(thumb) == 0 || len(thumb) > maxCatalogThumbBytes {
		t.Fatalf("thumbnail exceeds bound or is invalid: %d %v", len(thumb), err)
	}
	decoded, err := jpeg.Decode(bytes.NewReader(thumb))
	if err != nil {
		t.Fatal(err)
	}
	bounds := decoded.Bounds()
	if bounds.Dx() > 120 || bounds.Dy() > 120 || bounds.Dx() < 1 || bounds.Dy() < 1 {
		t.Fatalf("thumbnail size incorrect: %v", bounds)
	}
	r, g, b, _ := decoded.At(bounds.Min.X+bounds.Dx()/2, bounds.Min.Y+bounds.Dy()/2).RGBA()
	if r <= g*2 || r <= b*2 {
		t.Fatalf("thumbnail pixels do not reflect red source: r=%d g=%d b=%d", r, g, b)
	}
}

func TestCatalogThumbnailRejectsMalformedJPEG(t *testing.T) {
	if _, err := catalogThumbnailJPEG([]byte("not jpeg")); err == nil {
		t.Fatal("malformed image accepted")
	}
}
