package services

import (
	"testing"

	"github.com/oginoino/toca/toca-deal/internal/domain/document"
)

func TestShouldExtract(t *testing.T) {
	cases := []struct {
		name     string
		category document.Category
		fileType string
		want     bool
	}{
		{"identidade pdf", document.CategoryIdentidade, "application/pdf", true},
		{"comprovante imagem", document.CategoryComprovante, "image/png", true},
		{"certidao juridica pdf", document.CategoryJuridico, "application/pdf", true},
		{"contrato nao", document.CategoryContrato, "application/pdf", false},
		{"foto nao", document.CategoryFoto, "image/jpeg", false},
		{"identidade docx nao", document.CategoryIdentidade, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := &document.Document{Category: tc.category, FileType: tc.fileType}
			if got := shouldExtract(d); got != tc.want {
				t.Fatalf("shouldExtract(%s, %s) = %v, want %v", tc.category, tc.fileType, got, tc.want)
			}
		})
	}

	if shouldExtract(nil) {
		t.Fatal("shouldExtract(nil) = true, want false")
	}
}
