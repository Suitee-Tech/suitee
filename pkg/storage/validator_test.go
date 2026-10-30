package storage

import "testing"

func TestValidateWithMIMEReturnsDetectedImageType(t *testing.T) {
	t.Parallel()

	pngHeader := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	}

	ext, contentType, err := ValidateWithMIME(
		"toca-avatars",
		"avatar.bin",
		int64(len(pngHeader)),
		pngHeader,
	)
	if err != nil {
		t.Fatalf("ValidateWithMIME() error = %v", err)
	}
	if ext != ".png" {
		t.Fatalf("extension = %q, want .png", ext)
	}
	if contentType != "image/png" {
		t.Fatalf("content type = %q, want image/png", contentType)
	}
}

func TestValidateKeepsExtensionOnlyCompatibility(t *testing.T) {
	t.Parallel()

	jpegHeader := []byte{
		0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 0x4a, 0x46,
		0x49, 0x46, 0x00, 0x01, 0x01, 0x01, 0x00, 0x48,
	}

	ext, err := Validate(
		"toca-logos",
		"logo.upload",
		int64(len(jpegHeader)),
		jpegHeader,
	)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if ext != ".jpg" && ext != ".jpeg" {
		t.Fatalf("extension = %q, want JPEG extension", ext)
	}
}
