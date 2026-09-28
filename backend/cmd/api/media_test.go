package main

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAllowedImageType(t *testing.T) {
	for _, contentType := range []string{"image/jpeg", "image/png", "image/gif", "image/webp", "image/heic"} {
		if !allowedImageType(contentType) {
			t.Fatalf("%q must be accepted", contentType)
		}
	}
	if allowedImageType("application/pdf") {
		t.Fatal("non-image content type must be rejected")
	}
}

func TestMediaEndpointRequiresSession(t *testing.T) {
	s := NewServer(nil, true)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/media/85f35ef6-92a4-4dc8-97be-e844d7a58d4c", nil)
	w := httptest.NewRecorder()

	s.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("got %d, want %d", w.Code, http.StatusUnauthorized)
	}
	if got := w.Header().Get("Location"); got != "" {
		t.Fatalf("media must not redirect to object storage, got Location=%q", got)
	}
}

func TestMediaUploadRequiresSession(t *testing.T) {
	s := NewServer(nil, true)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/media", nil)
	w := httptest.NewRecorder()

	s.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("got %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestDetectUploadImageTypeRecognizesHEIC(t *testing.T) {
	data := append([]byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'h', 'e', 'i', 'c'}, make([]byte, 24)...)

	if got := detectUploadImageType(data); got != "image/heic" {
		t.Fatalf("got %q, want image/heic", got)
	}
}

func TestConvertImageToWebP(t *testing.T) {
	var source bytes.Buffer
	sourceImage := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	sourceImage.SetNRGBA(1, 1, color.NRGBA{R: 255, A: 255})
	if err := png.Encode(&source, sourceImage); err != nil {
		t.Fatal(err)
	}

	converted, err := convertImageToWebP(source.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if got := http.DetectContentType(converted); got != "image/webp" {
		t.Fatalf("got %q, want image/webp", got)
	}
}

func TestEncodeWebPVariants(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	lossless, err := encodeWebP(source, true, 100)
	if err != nil {
		t.Fatal(err)
	}
	optimized, err := encodeWebP(source, false, 75)
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"lossless": lossless, "optimized": optimized} {
		if got := http.DetectContentType(data); got != "image/webp" {
			t.Fatalf("%s = %q, want image/webp", name, got)
		}
	}
}

func TestConvertImageToWebPPreservesJPEGOrientation(t *testing.T) {
	var source bytes.Buffer
	if err := jpeg.Encode(&source, image.NewNRGBA(image.Rect(0, 0, 2, 3)), nil); err != nil {
		t.Fatal(err)
	}

	converted, err := convertImageToWebP(withEXIFOrientation(source.Bytes(), 6))
	if err != nil {
		t.Fatal(err)
	}
	decoded, _, err := image.Decode(bytes.NewReader(converted))
	if err != nil {
		t.Fatal(err)
	}
	if got := decoded.Bounds().Size(); got != (image.Point{X: 3, Y: 2}) {
		t.Fatalf("got %v, want portrait orientation rotated to 3x2", got)
	}
}

func withEXIFOrientation(jpegData []byte, orientation byte) []byte {
	payload := []byte{
		'E', 'x', 'i', 'f', 0, 0,
		'I', 'I', 42, 0, 8, 0, 0, 0,
		1, 0,
		0x12, 0x01, 3, 0, 1, 0, 0, 0, orientation, 0, 0, 0,
		0, 0, 0, 0,
	}
	app1 := append([]byte{0xff, 0xe1, 0, byte(len(payload) + 2)}, payload...)
	return append(append([]byte{}, jpegData[:2]...), append(app1, jpegData[2:]...)...)
}

func TestMediaObjectKeysKeepOriginalAndWebP(t *testing.T) {
	original, converted := mediaObjectKeys("space-1", "media-1", "image/heic")
	if original != "spaces/space-1/media/media-1.heic" {
		t.Fatalf("original key = %q", original)
	}
	if converted != "spaces/space-1/media/media-1.webp" {
		t.Fatalf("converted key = %q", converted)
	}
	if original == converted || !strings.HasSuffix(converted, ".webp") {
		t.Fatalf("keys must be distinct and converted key must be WebP: %q, %q", original, converted)
	}
}

func TestMediaObjectKeysKeepTwoWebPFiles(t *testing.T) {
	original, converted := mediaObjectKeys("space-1", "media-1", "image/webp")
	if original != "spaces/space-1/media/media-1.original.webp" || converted != "spaces/space-1/media/media-1.webp" {
		t.Fatalf("unexpected keys: %q, %q", original, converted)
	}
}

func TestMediaVariantKeysKeepLosslessAndOptimizedFiles(t *testing.T) {
	lossless, optimized := mediaVariantKeys("space-1", "media-1")
	if lossless != "spaces/space-1/media/media-1.lossless.webp" || optimized != "spaces/space-1/media/media-1.webp" {
		t.Fatalf("unexpected variant keys: %q, %q", lossless, optimized)
	}
}
