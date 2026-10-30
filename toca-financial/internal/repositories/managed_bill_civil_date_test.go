package repositories

import (
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/timeutil"
)

// TestCivilDateFromDB_KeepsManagedBillDueDate cobre o achado pré-existente:
// a coluna DATE volta do Postgres como meia-noite UTC; sem reanexar a data
// civil do tenant, a releitura de um managed bill existente desloca o
// vencimento em um dia e a re-afirmação idempotente do MESMO bill falha com
// "managed bill source key reused with different economic data".
func TestCivilDateFromDB_KeepsManagedBillDueDate(t *testing.T) {
	loc := timeutil.Location()

	// Vencimento do metadata do negócio (o mesmo valor gravado no primeiro
	// ensure): 2026-10-10T00:00:00Z — data civil 2026-10-09 em Brasília.
	chargeDueDate := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)

	// Write: o repositório normaliza para a data civil do tenant.
	civil := civilDateIn(loc, chargeDueDate)
	// Postgres DATE devolve meia-noite UTC do dia civil gravado.
	scannedFromDB := time.Date(civil.Year(), civil.Month(), civil.Day(), 0, 0, 0, 0, time.UTC)

	// Pré-condição do bug: SEM a normalização de leitura o vencimento lido cai
	// no dia civil anterior ao do metadata.
	if timeutil.StartOfDay(scannedFromDB).Equal(timeutil.StartOfDay(chargeDueDate)) {
		t.Fatalf("pré-condição falhou: sem normalização a data já coincidiria (lido=%s metadata=%s)",
			scannedFromDB, chargeDueDate)
	}

	// Com o fix, a leitura reconstrói a MESMA data civil do metadata.
	read := civilDateFromDB(loc, scannedFromDB)
	if !timeutil.StartOfDay(read).Equal(timeutil.StartOfDay(chargeDueDate)) {
		t.Fatalf("data civil divergente após a leitura: lido=%s metadata=%s", read, chargeDueDate)
	}

	// Idempotente: normalizar um valor já normalizado não desloca de novo.
	if again := civilDateFromDB(loc, read); !again.Equal(read) {
		t.Fatalf("civilDateFromDB não é idempotente: %s -> %s", read, again)
	}

	// Zero permanece zero (linhas legadas sem vencimento não viram 1970-01-01).
	if got := civilDateFromDB(loc, time.Time{}); !got.IsZero() {
		t.Fatalf("zero deveria permanecer zero, veio %s", got)
	}
}
