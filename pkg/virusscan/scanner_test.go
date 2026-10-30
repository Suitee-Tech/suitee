package virusscan

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// fakeClamd atende o protocolo zINSTREAM em um listener local e devolve o
// veredito configurado — também registra os bytes recebidos para asserção
// de "todo byte foi varrido".
type fakeClamd struct {
	verdict  string // "OK", "Eicar-Test-Signature FOUND", "garbage", "silent"
	received []byte
	// limit simula o StreamMaxLength do clamd: corta o stream e responde o
	// erro que o daemon real envia antes de fechar a conexão.
	limit int
	ln    net.Listener
	addr  string
}

func startFakeClamd(t *testing.T, verdict string) *fakeClamd {
	t.Helper()
	return startFakeClamdWithLimit(t, verdict, 0)
}

func startFakeClamdWithLimit(t *testing.T, verdict string, limit int) *fakeClamd {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	f := &fakeClamd{verdict: verdict, limit: limit, ln: ln, addr: ln.Addr().String()}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go f.handle(conn)
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return f
}

func (f *fakeClamd) handle(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	cmd, err := reader.ReadString('\x00')
	if err != nil {
		return
	}
	if strings.TrimSuffix(cmd, "\x00") != "zINSTREAM" {
		_, _ = writer.WriteString("UNKNOWN COMMAND\x00")
		_ = writer.Flush()
		return
	}
	// Protocolo real: 4 bytes de tamanho em ordem de rede antes de cada chunk
	// (clamd(8), INSTREAM). Um teste que lesse "<len>\n" mascararia o bug de
	// enquadramento que quebrava todo upload no beta.
	for {
		var header [4]byte
		if _, err := io.ReadFull(reader, header[:]); err != nil {
			return
		}
		size := int(binary.BigEndian.Uint32(header[:]))
		if size == 0 {
			break
		}
		if f.limit > 0 && len(f.received)+size > f.limit {
			// O clamd avisa e fecha a conexão imediatamente.
			_, _ = writer.WriteString("INSTREAM size limit exceeded. ERROR\x00")
			_ = writer.Flush()
			return
		}
		chunk := make([]byte, size)
		if _, err := io.ReadFull(reader, chunk); err != nil {
			return
		}
		f.received = append(f.received, chunk...)
	}
	switch f.verdict {
	case "silent":
		return // fecha sem responder — simula daemon que morreu no meio
	default:
		_, _ = writer.WriteString("stream: " + f.verdict + "\x00")
		_ = writer.Flush()
	}
}

func scanText(t *testing.T, addr, text string) (Result, error) {
	t.Helper()
	scanner := NewClamAVScanner(addr)
	scanner.ScanTimeout = 5 * time.Second
	return scanner.Scan(context.Background(), strings.NewReader(text))
}

func TestClamAVScanner_Clean(t *testing.T) {
	clamd := startFakeClamd(t, "OK")
	res, err := scanText(t, clamd.addr, "conteudo inofensivo de documento")
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if !res.Clean {
		t.Fatal("arquivo limpo marcado como infectado")
	}
	if string(clamd.received) != "conteudo inofensivo de documento" {
		t.Fatalf("clamd não recebeu todos os bytes: %q", clamd.received)
	}
}

func TestClamAVScanner_VirusFound(t *testing.T) {
	clamd := startFakeClamd(t, "Eicar-Test-Signature FOUND")
	_, err := scanText(t, clamd.addr, "payload")
	if !errors.Is(err, ErrVirusFound) {
		t.Fatalf("err = %v, want ErrVirusFound", err)
	}
	if !strings.Contains(err.Error(), "Eicar-Test-Signature") {
		t.Fatalf("assinatura ausente da mensagem: %v", err)
	}
}

func TestClamAVScanner_UnexpectedVerdictIsUnavailable(t *testing.T) {
	clamd := startFakeClamd(t, "garbage")
	_, err := scanText(t, clamd.addr, "payload")
	if !errors.Is(err, ErrScannerUnavailable) {
		t.Fatalf("err = %v, want ErrScannerUnavailable", err)
	}
}

func TestClamAVScanner_SilentDaemonIsUnavailable(t *testing.T) {
	clamd := startFakeClamd(t, "silent")
	_, err := scanText(t, clamd.addr, "payload")
	if !errors.Is(err, ErrScannerUnavailable) {
		t.Fatalf("err = %v, want ErrScannerUnavailable", err)
	}
}

func TestClamAVScanner_DialRefusedIsUnavailable(t *testing.T) {
	scanner := NewClamAVScanner("127.0.0.1:1") // nada escuta nessa porta
	scanner.DialTimeout = 500 * time.Millisecond
	_, err := scanner.Scan(context.Background(), strings.NewReader("x"))
	if !errors.Is(err, ErrScannerUnavailable) {
		t.Fatalf("err = %v, want ErrScannerUnavailable", err)
	}
}

func TestClamAVScanner_EmptyAddressIsUnavailable(t *testing.T) {
	_, err := NewClamAVScanner("  ").Scan(context.Background(), strings.NewReader("x"))
	if !errors.Is(err, ErrScannerUnavailable) {
		t.Fatalf("err = %v, want ErrScannerUnavailable", err)
	}
}

func TestClamAVScanner_MaxBytesFailsClosed(t *testing.T) {
	clamd := startFakeClamd(t, "OK")
	scanner := NewClamAVScanner(clamd.addr)
	scanner.MaxBytes = 4
	_, err := scanner.Scan(context.Background(), bytes.NewReader([]byte("12345")))
	if !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("err = %v, want ErrFileTooLarge (arquivo acima do teto)", err)
	}
	if errors.Is(err, ErrScannerUnavailable) {
		t.Fatal("arquivo grande não é indisponibilidade do scanner: o transporte responde 413, não 503")
	}
}

// Sem MaxBytes configurado o scanner não vira "sem limite": vale o teto do
// funil de upload.
func TestClamAVScanner_DefaultLimitIsTheUploadCeiling(t *testing.T) {
	clamd := startFakeClamd(t, "OK")
	scanner := NewClamAVScanner(clamd.addr)
	scanner.MaxBytes = 0
	if scanner.MaxBytes != 0 {
		t.Fatal("pré-condição do teste: MaxBytes zerado")
	}
	if MaxUploadBytes <= 0 {
		t.Fatal("teto do funil de upload precisa ser positivo")
	}
}

// O clamd corta o stream no StreamMaxLength dele e responde
// "INSTREAM size limit exceeded": isso é arquivo grande, não infraestrutura.
func TestClamAVScanner_ClamdSizeLimitIsFileTooLarge(t *testing.T) {
	clamd := startFakeClamdWithLimit(t, "OK", 32)
	scanner := NewClamAVScanner(clamd.addr)
	scanner.ChunkSize = 16
	_, err := scanner.Scan(context.Background(), strings.NewReader(strings.Repeat("a", 1024)))
	if !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("err = %v, want ErrFileTooLarge", err)
	}
}

// O enquadramento é o contrato com o clamd: 4 bytes de tamanho em ordem de rede
// antes de cada chunk e terminador de 4 bytes zero. Um teste que só olhasse os
// bytes recebidos não pegaria a regressão que quebrou o upload no beta.
func TestClamAVScanner_ChunkFramingIsNetworkByteOrder(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	raw := make(chan []byte, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		if _, err := reader.ReadString('\x00'); err != nil {
			return
		}
		var captured []byte
		var header [4]byte
		if _, err := io.ReadFull(reader, header[:]); err != nil {
			return
		}
		captured = append(captured, header[:]...)
		size := int(binary.BigEndian.Uint32(header[:]))
		chunk := make([]byte, size)
		if _, err := io.ReadFull(reader, chunk); err != nil {
			return
		}
		captured = append(captured, chunk...)
		if _, err := io.ReadFull(reader, header[:]); err != nil {
			return
		}
		captured = append(captured, header[:]...)
		raw <- captured
		_, _ = conn.Write([]byte("stream: OK\x00"))
	}()

	scanner := NewClamAVScanner(ln.Addr().String())
	scanner.ChunkSize = 4
	res, err := scanner.Scan(context.Background(), strings.NewReader("abcd"))
	if err != nil || !res.Clean {
		t.Fatalf("Scan = (%v, %v)", res, err)
	}
	got := <-raw
	want := []byte{0x00, 0x00, 0x00, 0x04, 'a', 'b', 'c', 'd', 0x00, 0x00, 0x00, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("enquadramento INSTREAM = % x, want % x", got, want)
	}
}

func TestClamAVScanner_MultiChunkStream(t *testing.T) {
	clamd := startFakeClamd(t, "OK")
	scanner := NewClamAVScanner(clamd.addr)
	scanner.ChunkSize = 7 // força vários chunks
	big := strings.Repeat("ab", 1000)
	res, err := scanner.Scan(context.Background(), strings.NewReader(big))
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if !res.Clean {
		t.Fatal("arquivo limpo marcado como infectado")
	}
	if string(clamd.received) != big {
		t.Fatalf("bytes divergentes: recebidos %d, enviados %d", len(clamd.received), len(big))
	}
}
