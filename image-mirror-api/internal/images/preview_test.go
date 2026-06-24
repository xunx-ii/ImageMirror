package images

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"testing"
)

func TestCreatePreviewDownscalesToMaxEdge(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 2048, 1024))
	draw.Draw(src, src.Bounds(), &image.Uniform{C: color.RGBA{R: 32, G: 96, B: 192, A: 255}}, image.Point{}, draw.Src)

	data := encodePNG(t, src)
	preview, err := createPreview(data, 512)
	if err != nil {
		t.Fatalf("createPreview returned error: %v", err)
	}

	decoded, _, err := image.Decode(bytes.NewReader(preview))
	if err != nil {
		t.Fatalf("preview is not decodable: %v", err)
	}
	if got, want := decoded.Bounds().Dx(), 512; got != want {
		t.Fatalf("preview width = %d, want %d", got, want)
	}
	if got, want := decoded.Bounds().Dy(), 256; got != want {
		t.Fatalf("preview height = %d, want %d", got, want)
	}
}

func TestCreatePreviewCompositesTransparencyOnWhite(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 16, 16))
	draw.Draw(src, src.Bounds(), &image.Uniform{C: color.RGBA{R: 255, A: 0}}, image.Point{}, draw.Src)

	data := encodePNG(t, src)
	preview, err := createPreview(data, 512)
	if err != nil {
		t.Fatalf("createPreview returned error: %v", err)
	}

	decoded, _, err := image.Decode(bytes.NewReader(preview))
	if err != nil {
		t.Fatalf("preview is not decodable: %v", err)
	}
	r, g, b, _ := decoded.At(0, 0).RGBA()
	if r < 62000 || g < 62000 || b < 62000 {
		t.Fatalf("transparent preview pixel = (%d, %d, %d), want near white", r, g, b)
	}
}

func encodePNG(t *testing.T, src image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatalf("png encode failed: %v", err)
	}
	return buf.Bytes()
}
