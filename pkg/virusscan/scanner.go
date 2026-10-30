// Package virusscan define o contrato de antivírus do funil de uploads
// (P1-13): todo byte que entra no storage passa por um Scanner antes de ser
// persistido. A implementação de produção fala o protocolo INSTREAM do
// clamd (ClamAV) por TCP; testes usam um clamd fake in-process.
//
// Semântica fail-closed: scanner indisponível (rede/timeout) ou ausente
// recusa o upload — nunca deixa passar um byte não verificado.
package virusscan

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

var (
	// ErrVirusFound marca um arquivo infectado. O Signature acompanha o nome
	// da assinatura detectada.
	ErrVirusFound = errors.New("virus found")
	// ErrScannerUnavailable marca falha de infraestrutura do scanner
	// (sem wiring, conexão recusada, timeout, resposta inválida).
	ErrScannerUnavailable = errors.New("virus scanner unavailable")
	// ErrFileTooLarge marca arquivo acima do teto de varredura. Não é falha de
	// infraestrutura: o transporte responde 413 e o usuário reduz o arquivo.
	ErrFileTooLarge = errors.New("file exceeds the scan limit")
)

// MaxUploadBytes é o teto de arquivo do funil de upload — o mesmo que o scanner
// varre. Exposto para o transporte recusar cedo (413), em vez de ler centenas
// de MB para falhar no fim. O `StreamMaxLength` do clamd precisa ser >= este
// valor, senão o próprio clamd corta o stream.
const MaxUploadBytes int64 = 25 << 20 // 25 MiB

// Result é o veredito de uma varredura.
type Result struct {
	Clean     bool
	Signature string // assinatura detectada (vazia quando Clean)
}

// Scanner varre um stream de bytes em busca de malware.
type Scanner interface {
	Scan(ctx context.Context, r io.Reader) (Result, error)
}

// ClamAVScanner fala o protocolo INSTREAM do clamd por TCP. Os parâmetros
// de rede são fixos e curtos de propósito: o funil de upload não pode
// esperar um daemon lento.
type ClamAVScanner struct {
	Addr string
	// MaxBytes é o teto de varredura: arquivos maiores são recusados
	// (fail-closed) em vez de varridos parcialmente.
	MaxBytes    int64
	DialTimeout time.Duration
	ScanTimeout time.Duration
	// ChunkSize é o tamanho dos chunks INSTREAM (clamd aceita até a
	// StreamMaxLength configurada; 64KB é conservador).
	ChunkSize int
}

// NewClamAVScanner monta o scanner padrão para o endereço do clamd
// (ex.: CLAMAV_URL="clamav:3310").
func NewClamAVScanner(addr string) *ClamAVScanner {
	return &ClamAVScanner{
		Addr:        addr,
		MaxBytes:    MaxUploadBytes,
		DialTimeout: 3 * time.Second,
		ScanTimeout: 60 * time.Second,
		ChunkSize:   64 << 10,
	}
}

// Scan implementa o protocolo zINSTREAM do clamd (clamd(8)):
//
//	cliente → "zINSTREAM\0"
//	cliente → "<len:4 bytes big-endian><bytes>" repetido
//	cliente → "<0:4 bytes>"                 (terminador)
//	clamd   → "stream: OK\0" | "stream: <sig> FOUND\0"
//
// O tamanho do chunk é um inteiro de 4 bytes em ordem de rede — NÃO um decimal
// em ASCII. Com o formato errado o clamd lê os primeiros 4 bytes do texto
// ("6553" de "65536\n") como um chunk de ~867 MiB, responde "INSTREAM size
// limit exceeded" e fecha a conexão: todo upload passava a falhar como
// "scanner indisponível" (issue "erro ao enviar documento da parte").
func (s *ClamAVScanner) Scan(ctx context.Context, r io.Reader) (Result, error) {
	if strings.TrimSpace(s.Addr) == "" {
		return Result{}, fmt.Errorf("%w: address not configured", ErrScannerUnavailable)
	}

	dialer := net.Dialer{Timeout: s.DialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", s.Addr)
	if err != nil {
		return Result{}, fmt.Errorf("%w: dial %s: %v", ErrScannerUnavailable, s.Addr, err)
	}
	defer conn.Close()

	if s.ScanTimeout > 0 {
		_ = conn.SetDeadline(time.Now().Add(s.ScanTimeout))
	}

	writer := bufio.NewWriter(conn)
	reader := bufio.NewReader(conn)

	if _, err := writer.WriteString("zINSTREAM\x00"); err != nil {
		return Result{}, fmt.Errorf("%w: send command: %v", ErrScannerUnavailable, err)
	}

	chunkSize := s.ChunkSize
	if chunkSize <= 0 || chunkSize > 1<<20 {
		chunkSize = 64 << 10
	}
	// O teto local vem da configuração do scanner; sem ela vale o teto do
	// funil de upload (nunca "sem limite").
	maxBytes := s.MaxBytes
	if maxBytes <= 0 {
		maxBytes = MaxUploadBytes
	}

	buf := make([]byte, chunkSize)
	var scanned int64
	for {
		n, readErr := io.ReadFull(r, buf)
		if n > 0 {
			scanned += int64(n)
			if scanned > maxBytes {
				return Result{}, fmt.Errorf("%w: %d bytes (limite %d)", ErrFileTooLarge, scanned, maxBytes)
			}
			if err := writeChunk(writer, buf[:n], ErrScannerUnavailable); err != nil {
				return Result{}, err
			}
		}
		if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
			break
		}
		if readErr != nil {
			return Result{}, fmt.Errorf("%w: read upload stream: %v", ErrScannerUnavailable, readErr)
		}
	}
	if err := writeChunk(writer, nil, ErrScannerUnavailable); err != nil {
		return Result{}, err
	}
	if err := writer.Flush(); err != nil {
		return Result{}, fmt.Errorf("%w: flush stream: %v", ErrScannerUnavailable, err)
	}

	line, err := reader.ReadString('\x00')
	if err != nil && err != io.EOF {
		return Result{}, fmt.Errorf("%w: read verdict: %v", ErrScannerUnavailable, err)
	}
	line = strings.TrimSuffix(line, "\x00")
	line = strings.TrimSpace(line)
	// clamd pode devolver o prefixo "stream:" ou ficar em silêncio total
	// (RESPONSE "stream: OK" sem NUL em configurações antigas) — os dois
	// casos são tratados; qualquer outra resposta é falha de infra.
	switch {
	case line == "stream: OK", strings.HasSuffix(line, "OK"):
		return Result{Clean: true}, nil
	case strings.HasSuffix(line, "FOUND"):
		signature := strings.TrimSpace(strings.TrimSuffix(line, "FOUND"))
		signature = strings.TrimSpace(strings.TrimPrefix(signature, "stream:"))
		return Result{Clean: false, Signature: signature}, fmt.Errorf("%w: %s", ErrVirusFound, signature)
	case strings.Contains(strings.ToLower(line), "size limit"):
		// O clamd corta o stream no StreamMaxLength dele: o arquivo é grande
		// demais para ser varrido, não é indisponibilidade do serviço.
		return Result{}, fmt.Errorf("%w: clamd %q", ErrFileTooLarge, line)
	default:
		return Result{}, fmt.Errorf("%w: unexpected verdict %q", ErrScannerUnavailable, line)
	}
}

// writeChunk escreve um chunk do INSTREAM: 4 bytes de tamanho em ordem de rede
// seguidos dos dados. Um chunk vazio é o terminador do stream.
func writeChunk(w *bufio.Writer, chunk []byte, wrap error) error {
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(chunk)))
	if _, err := w.Write(header[:]); err != nil {
		return fmt.Errorf("%w: send chunk size: %v", wrap, err)
	}
	if len(chunk) == 0 {
		return nil
	}
	if _, err := w.Write(chunk); err != nil {
		return fmt.Errorf("%w: send chunk: %v", wrap, err)
	}
	return nil
}
