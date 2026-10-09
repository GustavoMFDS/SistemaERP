package handlers

import (
    "bytes"
    "encoding/base64"
    "image"
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
        "empty": "",
        "invalid base64": "!!!",
        "html": base64.StdEncoding.EncodeToString([]byte("<script>alert(1)</script>")),
        "wrong file": base64.StdEncoding.EncodeToString([]byte("GIF89a")),
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
    if err != nil { t.Fatal(err) }
    suffix := []byte("<svg onload=alert(1)>")
    raw = append(raw, suffix...)
    clean, err := catalogJPEG(base64.StdEncoding.EncodeToString(raw), maxCatalogPhotoBytes, 1600)
    // A strict decoder may reject trailing bytes entirely. Either rejection
    // or safe re-encoding is acceptable; retaining the suffix is not.
    if err == nil && bytes.Contains(clean, suffix) {
        t.Fatal("untrusted suffix survived JPEG recompression")
    }
}
