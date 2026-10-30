package storage

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

// makeTestImage produz um image.Image em memória das dimensões pedidas.
// Cada pixel tem cor única (mistura RGB a partir de x,y) para que o
// resize não vire uma matriz uniforme — assim conseguimos detectar
// problemas de stride/scanline.
func makeTestImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8(x % 256),
				G: uint8(y % 256),
				B: uint8((x + y) % 256),
				A: 255,
			})
		}
	}
	return img
}

func encodeJPEG(t *testing.T, img image.Image, quality int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		t.Fatalf("jpeg.Encode: %v", err)
	}
	return buf.Bytes()
}

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

func TestProcessImage_ShrinksWhenLargerThanMax(t *testing.T) {
	// 4000x3000 → max 1920 → deve virar 1920x1440 (mantém aspect ratio
	// 4:3). Em jpeg85% deve gerar menos bytes que o original em q90.
	original := encodeJPEG(t, makeTestImage(4000, 3000), 90)

	out, w, h, err := ProcessImage(original, DefaultPhotoOptions)
	if err != nil {
		t.Fatalf("ProcessImage: %v", err)
	}
	if w != 1920 || h != 1440 {
		t.Errorf("expected 1920x1440, got %dx%d", w, h)
	}
	if len(out) >= len(original) {
		t.Errorf("expected resized to be smaller than original (%d vs %d)", len(out), len(original))
	}
}

func TestProcessImage_NoUpscalingWhenSmaller(t *testing.T) {
	// 800x600 < max 1920 → mantém dimensões originais (sem upscale).
	// Re-encoda em JPEG 85% e devolve.
	original := encodeJPEG(t, makeTestImage(800, 600), 95)

	_, w, h, err := ProcessImage(original, DefaultPhotoOptions)
	if err != nil {
		t.Fatalf("ProcessImage: %v", err)
	}
	if w != 800 || h != 600 {
		t.Errorf("expected dimensions preserved (800x600), got %dx%d", w, h)
	}
}

func TestProcessImage_PortraitOrientationPreservesAspect(t *testing.T) {
	// Imagem em retrato: 1500x2500 → maior dim é 2500 (altura). Max
	// 1920 → altura vira 1920, largura vira 1500*(1920/2500) ≈ 1152.
	original := encodeJPEG(t, makeTestImage(1500, 2500), 90)

	_, w, h, err := ProcessImage(original, DefaultPhotoOptions)
	if err != nil {
		t.Fatalf("ProcessImage: %v", err)
	}
	if h != 1920 {
		t.Errorf("expected new height=1920 (largest dim), got %d", h)
	}
	expectedW := 1152
	if w < expectedW-2 || w > expectedW+2 {
		t.Errorf("expected new width≈%d, got %d", expectedW, w)
	}
}

func TestProcessImage_AcceptsPNG(t *testing.T) {
	// Upload do app pode chegar como PNG (screenshots). Aceitar e
	// converter para JPEG via re-encode.
	original := encodePNG(t, makeTestImage(2400, 1600))

	out, w, h, err := ProcessImage(original, DefaultPhotoOptions)
	if err != nil {
		t.Fatalf("ProcessImage(png): %v", err)
	}
	if w != 1920 {
		t.Errorf("expected new width=1920, got %d", w)
	}
	// Output must be decodable as JPEG (re-encoded).
	if _, err := jpeg.Decode(bytes.NewReader(out)); err != nil {
		t.Errorf("expected output to be valid JPEG: %v", err)
	}
	_ = h
}

func TestProcessImage_EmptyInputRejected(t *testing.T) {
	_, _, _, err := ProcessImage(nil, DefaultPhotoOptions)
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestProcessImage_GarbageInputRejected(t *testing.T) {
	// Bytes que não são imagem nenhuma: image.Decode falha.
	_, _, _, err := ProcessImage([]byte("not an image"), DefaultPhotoOptions)
	if err == nil {
		t.Error("expected decode error for garbage bytes")
	}
}

func TestProcessImage_AppliesDefaultsWhenZeroOptions(t *testing.T) {
	// MaxDimensionPx=0 e JPEGQuality=0 → preenche com defaults (1920, 85).
	original := encodeJPEG(t, makeTestImage(2000, 1500), 95)
	out, w, _, err := ProcessImage(original, ResizeOptions{})
	if err != nil {
		t.Fatalf("ProcessImage: %v", err)
	}
	if w != 1920 {
		t.Errorf("expected default max 1920, got %d", w)
	}
	if len(out) == 0 {
		t.Error("expected non-empty output")
	}
}

func TestProcessImage_ThumbnailDimensions(t *testing.T) {
	// 4000x2000 com thumbnail (320 max). Esperado 320x160.
	original := encodeJPEG(t, makeTestImage(4000, 2000), 90)
	_, w, h, err := ProcessImage(original, DefaultThumbnailOptions)
	if err != nil {
		t.Fatalf("ProcessImage thumb: %v", err)
	}
	if w != 320 || h != 160 {
		t.Errorf("expected thumbnail 320x160, got %dx%d", w, h)
	}
}

func TestResizeKeepAspect_NoOpWhenAlreadySmaller(t *testing.T) {
	// Sanity check da função interna: 100x200 com max=500 → retorna
	// a mesma imagem (sem upscaling). Compara bounds.
	src := makeTestImage(100, 200)
	out := resizeKeepAspect(src, 500)
	if out.Bounds() != src.Bounds() {
		t.Errorf("expected bounds unchanged, got %v vs %v", out.Bounds(), src.Bounds())
	}
}
