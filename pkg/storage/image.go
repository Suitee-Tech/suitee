package storage

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // registra decoder PNG para image.Decode aceitar uploads .png

	"golang.org/x/image/draw"
)

// image.go — pipeline de processamento de imagens para uploads de
// vistoria (issue #440) e qualquer outro upload que precise de
// compressão server-side antes de persistir.
//
// Stdlib `image` decoda JPEG/PNG/GIF, mas o resize bom de qualidade
// (CatmullRom, bilinear) vive em `golang.org/x/image/draw` — pequena
// dep mantida pelo time do Go. Sem isso, teríamos que reimplementar
// nearest-neighbor manualmente, perdendo qualidade visual.
//
// As funções aqui são puras: recebem bytes, devolvem bytes. Quem chama
// (InspectionService) faz upload via storage.Service.

// ResizeOptions parametriza o processamento. Defaults vivem no caller —
// para vistoria usamos MaxDimensionPx=1920, JPEGQuality=85 conforme
// audit J04 #440. Para thumbnails, MaxDimensionPx=320.
type ResizeOptions struct {
	// MaxDimensionPx é a maior dimensão (largura OU altura) permitida.
	// Imagem é redimensionada para caber preservando aspect ratio. Se
	// a imagem já é menor que MaxDimensionPx em ambas dimensões, passa
	// direto sem upscaling.
	MaxDimensionPx int
	// JPEGQuality 0-100. Recomendado 85 para fotos (sweet spot
	// tamanho/qualidade no perfil de cobrança vs. inspeção).
	JPEGQuality int
}

// ProcessImage decoda src (JPEG ou PNG), redimensiona se necessário e
// re-encoda como JPEG no nível de qualidade pedido. Retorna os bytes
// processados prontos para upload, mais width/height finais para
// metadata.
//
// Erros:
//   - bytes não decodificam como imagem → erro descritivo
//   - encoding JPEG falha (raríssimo) → erro embrulha o stdlib
func ProcessImage(src []byte, opts ResizeOptions) (out []byte, width, height int, err error) {
	if len(src) == 0 {
		return nil, 0, 0, fmt.Errorf("empty image bytes")
	}
	if opts.MaxDimensionPx <= 0 {
		opts.MaxDimensionPx = 1920
	}
	if opts.JPEGQuality <= 0 || opts.JPEGQuality > 100 {
		opts.JPEGQuality = 85
	}

	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, 0, 0, fmt.Errorf("decode image: %w", err)
	}

	resized := resizeKeepAspect(img, opts.MaxDimensionPx)
	bounds := resized.Bounds()

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, resized, &jpeg.Options{Quality: opts.JPEGQuality}); err != nil {
		return nil, 0, 0, fmt.Errorf("encode jpeg: %w", err)
	}
	return buf.Bytes(), bounds.Dx(), bounds.Dy(), nil
}

// resizeKeepAspect aplica CatmullRom (bicubic) quando a imagem precisa
// shrink. Mantém a relação de aspecto exata.
//
// Não faz upscaling: se a imagem já é menor que maxDim, devolve a
// original re-encodada (re-encode ainda passa pelo JPEG quality, então
// a função na chamada gera bytes consistentes).
func resizeKeepAspect(src image.Image, maxDim int) image.Image {
	srcBounds := src.Bounds()
	srcW, srcH := srcBounds.Dx(), srcBounds.Dy()
	if srcW <= maxDim && srcH <= maxDim {
		// Já cabe: retorna como está. O caller ainda re-encoda para
		// JPEG na qualidade desejada.
		return src
	}

	// Calcula nova dimensão preservando aspect ratio.
	var newW, newH int
	if srcW >= srcH {
		newW = maxDim
		newH = int(float64(srcH) * (float64(maxDim) / float64(srcW)))
	} else {
		newH = maxDim
		newW = int(float64(srcW) * (float64(maxDim) / float64(srcH)))
	}

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	// CatmullRom: bicubic com qualidade visual alta, custo maior que
	// BiLinear. Para fotos de vistoria (uma vez upload, vista N vezes)
	// vale o custo de CPU no upload.
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, srcBounds, draw.Over, nil)
	return dst
}

// DefaultPhotoOptions e DefaultThumbnailOptions são os presets que o
// caller normalmente quer. Issue #440 fixa 1920px/q85 para a foto
// principal; thumbnail 320px/q75 é suficiente para preview no admin.
var (
	DefaultPhotoOptions     = ResizeOptions{MaxDimensionPx: 1920, JPEGQuality: 85}
	DefaultThumbnailOptions = ResizeOptions{MaxDimensionPx: 320, JPEGQuality: 75}
)
