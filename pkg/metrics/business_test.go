package metrics

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// Fase 1.3 do plano de inadimplência — as séries do domínio de cobrança
// precisam existir no /metrics, com rótulo fechado e sem id de entidade
// (cardinalidade alta é o que derruba o Prometheus em produção).

const testTenant = "11111111-1111-1111-1111-111111111111"

func scrapeMetrics(t *testing.T) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/metrics", Handler())

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("scrape devolveu status %d", recorder.Code)
	}
	return recorder.Body.String()
}

// sample devolve o valor de uma série exata (nome + labels literais).
func sample(t *testing.T, body, series string) (float64, bool) {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, series+" ") {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimPrefix(line, series+" ")), 64)
		if err != nil {
			t.Fatalf("valor inválido em %q: %v", line, err)
		}
		return v, true
	}
	return 0, false
}

func mustSample(t *testing.T, body, series string) float64 {
	t.Helper()
	v, ok := sample(t, body, series)
	if !ok {
		t.Fatalf("série ausente no /metrics: %s", series)
	}
	return v
}

func TestOverdueSnapshotGaugesAreExported(t *testing.T) {
	SetOverdueSnapshot(testTenant, 12345.67, 7)

	body := scrapeMetrics(t)
	if !strings.Contains(body, "# TYPE financial_overdue_amount_brl gauge") {
		t.Error("financial_overdue_amount_brl sem declaração de tipo (gauge)")
	}
	if !strings.Contains(body, "# TYPE financial_overdue_charges gauge") {
		t.Error("financial_overdue_charges sem declaração de tipo (gauge)")
	}

	if got := mustSample(t, body, `financial_overdue_amount_brl{tenant_id="`+testTenant+`"}`); got != 12345.67 {
		t.Errorf("valor vencido = %v, queria 12345.67", got)
	}
	if got := mustSample(t, body, `financial_overdue_charges{tenant_id="`+testTenant+`"}`); got != 7 {
		t.Errorf("cobranças vencidas = %v, queria 7", got)
	}

	// Segundo retrato do mesmo tenant substitui o primeiro (gauge, não soma).
	SetOverdueSnapshot(testTenant, 10, 1)
	body = scrapeMetrics(t)
	if got := mustSample(t, body, `financial_overdue_amount_brl{tenant_id="`+testTenant+`"}`); got != 10 {
		t.Errorf("retrato não foi substituído: %v", got)
	}
	if got := mustSample(t, body, `financial_overdue_charges{tenant_id="`+testTenant+`"}`); got != 1 {
		t.Errorf("contagem não foi substituída: %v", got)
	}
}

func TestOverdueSnapshotDropsEmptyTenantAndNegativeValues(t *testing.T) {
	SetOverdueSnapshot("", 999, 9)
	if strings.Contains(scrapeMetrics(t), `financial_overdue_amount_brl{tenant_id=""}`) {
		t.Error("tenant vazio virou série (bug do chamador não pode poluir o /metrics)")
	}

	SetOverdueSnapshot("22222222-2222-2222-2222-222222222222", -5, -3)
	body := scrapeMetrics(t)
	if got := mustSample(t, body, `financial_overdue_amount_brl{tenant_id="22222222-2222-2222-2222-222222222222"}`); got != 0 {
		t.Errorf("valor negativo deveria ser 0, veio %v", got)
	}
	if got := mustSample(t, body, `financial_overdue_charges{tenant_id="22222222-2222-2222-2222-222222222222"}`); got != 0 {
		t.Errorf("contagem negativa deveria ser 0, veio %v", got)
	}
}

func TestCollectionStatusCountsIsASnapshot(t *testing.T) {
	SetCollectionStatusCounts(testTenant, map[string]float64{
		"ACTIVE":              2,
		"JUDICIAL":            1,
		"guarantee_activated": 3,
	})

	body := scrapeMetrics(t)
	if !strings.Contains(body, "# TYPE financial_collections_by_status gauge") {
		t.Error("financial_collections_by_status sem declaração de tipo (gauge)")
	}
	// O enum do domínio é maiúsculo; o rótulo sai minúsculo (convenção do arquivo).
	if got := mustSample(t, body, `financial_collections_by_status{tenant_id="`+testTenant+`",status="active"}`); got != 2 {
		t.Errorf("status active = %v, queria 2", got)
	}
	if got := mustSample(t, body, `financial_collections_by_status{tenant_id="`+testTenant+`",status="judicial"}`); got != 1 {
		t.Errorf("status judicial = %v, queria 1", got)
	}
	if got := mustSample(t, body, `financial_collections_by_status{tenant_id="`+testTenant+`",status="guarantee_activated"}`); got != 3 {
		t.Errorf("status guarantee_activated = %v, queria 3", got)
	}

	// Retrato novo sem JUDICIAL: a série tem de sumir, não ficar congelada.
	SetCollectionStatusCounts(testTenant, map[string]float64{"ACTIVE": 5})
	body = scrapeMetrics(t)
	if got := mustSample(t, body, `financial_collections_by_status{tenant_id="`+testTenant+`",status="active"}`); got != 5 {
		t.Errorf("status active = %v, queria 5", got)
	}
	if strings.Contains(body, `financial_collections_by_status{tenant_id="`+testTenant+`",status="judicial"}`) {
		t.Error("status ausente do retrato continua exposto (gauge virou acumulador)")
	}
}

func TestOpenCollectionDebtGaugeIsExported(t *testing.T) {
	SetOpenCollectionDebtBRL(testTenant, 9876.5)

	body := scrapeMetrics(t)
	if !strings.Contains(body, "# TYPE financial_collection_case_total_debt_brl gauge") {
		t.Error("financial_collection_case_total_debt_brl sem declaração de tipo (gauge)")
	}
	if got := mustSample(t, body, `financial_collection_case_total_debt_brl{tenant_id="`+testTenant+`"}`); got != 9876.5 {
		t.Errorf("dívida dos casos abertos = %v, queria 9876.5", got)
	}
	SetOpenCollectionDebtBRL("", 1)
	if strings.Contains(scrapeMetrics(t), `financial_collection_case_total_debt_brl{tenant_id=""}`) {
		t.Error("tenant vazio virou série")
	}
}

func TestDelinquencyTickCountsResultAndObservesDuration(t *testing.T) {
	const (
		okSeries  = `financial_delinquency_tick_total{result="success"}`
		errSeries = `financial_delinquency_tick_total{result="error"}`
		countS    = `financial_delinquency_tick_duration_seconds_count`
		sumS      = `financial_delinquency_tick_duration_seconds_sum`
		bucket05  = `financial_delinquency_tick_duration_seconds_bucket{le="0.5"}`
		bucket2   = `financial_delinquency_tick_duration_seconds_bucket{le="2"}`
		bucket5   = `financial_delinquency_tick_duration_seconds_bucket{le="5"}`
	)

	before := scrapeMetrics(t)
	if !strings.Contains(before, "# TYPE financial_delinquency_tick_total counter") {
		t.Error("financial_delinquency_tick_total sem declaração de tipo (counter)")
	}
	if !strings.Contains(before, "# TYPE financial_delinquency_tick_duration_seconds histogram") {
		t.Error("financial_delinquency_tick_duration_seconds sem declaração de tipo (histogram)")
	}
	// Os dois resultados existem desde o boot, mesmo zerados: é o que dá
	// série ao alerta de "nenhum tick bem-sucedido na janela".
	if _, ok := sample(t, before, okSeries); !ok {
		t.Error("result=success não é emitido antes da primeira observação")
	}
	if _, ok := sample(t, before, errSeries); !ok {
		t.Error("result=error não é emitido antes da primeira observação")
	}

	ObserveDelinquencyTick("success", 3.2)
	ObserveDelinquencyTick("SUCCESS", 0.4)
	ObserveDelinquencyTick("error", 1.0)
	ObserveDelinquencyTick("", 1.0)          // resultado vazio é bug do chamador: conta como erro
	ObserveDelinquencyTick("boom", 99999999) // duração absurda não deve estourar a conversão

	after := scrapeMetrics(t)
	if got := mustSample(t, after, okSeries) - mustSample(t, before, okSeries); got != 2 {
		t.Errorf("delta de ticks com sucesso = %v, queria 2", got)
	}
	if got := mustSample(t, after, errSeries) - mustSample(t, before, errSeries); got != 3 {
		t.Errorf("delta de ticks com erro = %v, queria 3", got)
	}
	if got := mustSample(t, after, countS) - mustSample(t, before, countS); got != 4 {
		t.Errorf("observações no histograma = %v, queria 4 (duração absurda é descartada)", got)
	}
	if got := mustSample(t, after, bucket05) - mustSample(t, before, bucket05); got != 1 {
		t.Errorf("bucket le=0.5 ganhou %v observações, queria 1", got)
	}
	if got := mustSample(t, after, bucket2) - mustSample(t, before, bucket2); got != 3 {
		t.Errorf("bucket le=2 ganhou %v observações, queria 3", got)
	}
	if got := mustSample(t, after, bucket5) - mustSample(t, before, bucket5); got != 4 {
		t.Errorf("bucket le=5 ganhou %v observações, queria 4", got)
	}
	if got := mustSample(t, after, sumS) - mustSample(t, before, sumS); got < 5.5 || got > 5.7 {
		t.Errorf("soma das durações = %v, queria ~5.6", got)
	}
	// Buckets próprios: o topo é 10 minutos, não os 5s da latência HTTP.
	if strings.Contains(after, `financial_delinquency_tick_duration_seconds_bucket{le="0.005"}`) {
		t.Error("histograma do tick usando os buckets padrão de latência HTTP")
	}
	if !strings.Contains(after, `financial_delinquency_tick_duration_seconds_bucket{le="+Inf"}`) {
		t.Error("histograma do tick sem bucket +Inf")
	}
}

func TestDelinquencyManualRunsCounter(t *testing.T) {
	const (
		okSeries  = `financial_delinquency_manual_runs_total{result="success"}`
		errSeries = `financial_delinquency_manual_runs_total{result="error"}`
	)
	before := scrapeMetrics(t)
	if !strings.Contains(before, "# TYPE financial_delinquency_manual_runs_total counter") {
		t.Error("financial_delinquency_manual_runs_total sem declaração de tipo (counter)")
	}

	IncDelinquencyManualRun("success")
	IncDelinquencyManualRun("error")
	IncDelinquencyManualRun("falhou") // enum fechado: qualquer coisa ≠ success vira error

	after := scrapeMetrics(t)
	if got := mustSample(t, after, okSeries) - mustSample(t, before, okSeries); got != 1 {
		t.Errorf("delta de execuções manuais com sucesso = %v, queria 1", got)
	}
	if got := mustSample(t, after, errSeries) - mustSample(t, before, errSeries); got != 2 {
		t.Errorf("delta de execuções manuais com erro = %v, queria 2", got)
	}
}

// TestFinancialMetricsKeepCardinalityBounded é o guarda-corpo do contrato:
// nenhuma série do domínio pode ganhar label de entidade (collection_id,
// charge_id, contract_id, user_id...) — isso explode o armazenamento do
// Prometheus e é exatamente o que o arquivo proíbe no topo.
func TestFinancialMetricsKeepCardinalityBounded(t *testing.T) {
	SetOverdueSnapshot(testTenant, 1500.25, 4)
	SetCollectionStatusCounts(testTenant, map[string]float64{"ACTIVE": 2, "JUDICIAL": 1, "RESOLVED": 9})
	SetOpenCollectionDebtBRL(testTenant, 3200)
	ObserveDelinquencyTick("success", 1.5)
	IncDelinquencyManualRun("success")

	families := []string{
		"financial_overdue_amount_brl",
		"financial_overdue_charges",
		"financial_collections_by_status",
		"financial_collection_case_total_debt_brl",
		"financial_delinquency_tick_total",
		"financial_delinquency_tick_duration_seconds",
		"financial_delinquency_manual_runs_total",
	}
	allowed := map[string]bool{"tenant_id": true, "status": true, "result": true, "le": true}
	forbidden := []string{"collection_id", "charge_id", "contract_id", "user_id", "case_id", "debtor_id", "tenant"}

	body := scrapeMetrics(t)
	seen := map[string]bool{}
	for _, line := range strings.Split(body, "\n") {
		name := metricName(line)
		family := ""
		for _, f := range families {
			// _bucket/_sum/_count pertencem à mesma família do histograma.
			if name == f || strings.HasPrefix(name, f+"_") {
				family = f
				break
			}
		}
		if family == "" {
			continue
		}
		seen[family] = true
		for _, label := range labelNames(line) {
			if !allowed[label] {
				t.Errorf("label %q não permitido em %q", label, line)
			}
			if contains(forbidden, label) {
				t.Errorf("label de entidade em %q", line)
			}
		}
	}
	for _, f := range families {
		if !seen[f] {
			t.Errorf("família %s não foi exposta no scrape", f)
		}
	}

	// O counter do motor é global de propósito: se um dia ganhar tenant_id,
	// o alerta e a cardinalidade mudam sem ninguém perceber.
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "financial_delinquency_tick_total{") && strings.Contains(line, "tenant_id") {
			t.Errorf("financial_delinquency_tick_total ganhou tenant_id: %q", line)
		}
	}
}

func TestDelinquencyLabelsAreEscaped(t *testing.T) {
	SetOverdueSnapshot(`tenant"quebrado`, 1, 1)

	body := scrapeMetrics(t)
	if !strings.Contains(body, `financial_overdue_amount_brl{tenant_id="tenant\"quebrado"} 1`) {
		t.Error("valor de label não foi escapado — o texto Prometheus fica inválido")
	}
}

// TestExpositionFormatIsParseable valida linha a linha o formato de exposição
// (nome, labels entre aspas, valor numérico) de tudo que é emitido.
func TestExpositionFormatIsParseable(t *testing.T) {
	SetOverdueSnapshot(testTenant, 12.5, 1)
	SetCollectionStatusCounts(testTenant, map[string]float64{"ACTIVE": 1})
	SetOpenCollectionDebtBRL(testTenant, 12.5)
	ObserveDelinquencyTick("success", 2)
	IncDelinquencyManualRun("error")

	line := regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*(\{[^}]*\})? [^ ]+$`)
	label := regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*="(?:[^"\\]|\\.)*"$`)
	number := regexp.MustCompile(`^(NaN|[+-]?Inf|[+-]?[0-9]*\.?[0-9]+([eE][+-]?[0-9]+)?)$`)

	for _, l := range strings.Split(scrapeMetrics(t), "\n") {
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		if !line.MatchString(l) {
			t.Fatalf("linha fora do formato Prometheus: %q", l)
		}
		parts := strings.SplitN(l, " ", 2)
		if !number.MatchString(parts[1]) {
			t.Fatalf("valor não numérico: %q", l)
		}
		if i := strings.Index(l, "{"); i >= 0 {
			labels := l[i+1 : strings.LastIndex(l, "}")]
			if labels == "" {
				t.Fatalf("conjunto de labels vazio: %q", l)
			}
			for _, pair := range splitPairs(labels) {
				if !label.MatchString(pair) {
					t.Fatalf("label fora do formato: %q (linha %q)", pair, l)
				}
			}
		}
	}
}

// TestHTTPHistogramKeepsDefaultBuckets protege a mudança em metrics.go: só o
// histograma do tick tem limites próprios; HTTP e eventos seguem nos buckets
// padrão de latência.
func TestHTTPHistogramKeepsDefaultBuckets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(GinMiddleware("toca-financial"))
	engine.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })
	engine.GET("/metrics", Handler())

	engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ping", nil))

	body := scrapeMetrics(t)
	for _, le := range []string{"0.005", "0.1", "5", "+Inf"} {
		// formatLabels ordena as chaves, então "le" vem primeiro.
		want := `http_request_duration_seconds_bucket{le="` + le + `",method="GET",path="/ping",service="toca-financial",status="200"}`
		if _, ok := sample(t, body, want); !ok {
			t.Errorf("bucket padrão ausente: %s", want)
		}
	}
}

// TestDefaultHistogramBoundsUnchanged garante que newHistogram continua com os
// buckets padrão (o caminho novo é opt-in).
func TestDefaultHistogramBoundsUnchanged(t *testing.T) {
	if got := len(newHistogram().limits()); got != len(buckets) {
		t.Fatalf("newHistogram com %d limites, queria %d", got, len(buckets))
	}
	custom := newHistogramWithBounds([]float64{1, 10})
	if got := len(custom.limits()); got != 2 {
		t.Fatalf("newHistogramWithBounds com %d limites, queria 2", got)
	}
	if got := len(custom.counts); got != 3 {
		t.Fatalf("counts do histograma customizado = %d, queria 3 (2 limites + +Inf)", got)
	}
	// A cópia evita que o chamador mute os limites depois de criar.
	src := []float64{1, 10}
	h := newHistogramWithBounds(src)
	src[0] = 99
	if h.limits()[0] != 1 {
		t.Error("limites foram compartilhados com o slice do chamador")
	}
	// Observe e leitura precisam usar a mesma régua.
	h.observe(1500 * time.Millisecond)
	if h.counts[0].Load() != 0 || h.counts[1].Load() != 1 || h.counts[2].Load() != 0 {
		t.Errorf("observação caiu no bucket errado: %d/%d/%d", h.counts[0].Load(), h.counts[1].Load(), h.counts[2].Load())
	}
}

func metricName(line string) string {
	for i := 0; i < len(line); i++ {
		if line[i] == '{' || line[i] == ' ' {
			return line[:i]
		}
	}
	return line
}

func labelNames(line string) []string {
	i := strings.Index(line, "{")
	if i < 0 {
		return nil
	}
	j := strings.LastIndex(line, "}")
	if j < i {
		return nil
	}
	var names []string
	for _, pair := range splitPairs(line[i+1 : j]) {
		if eq := strings.Index(pair, "="); eq > 0 {
			names = append(names, pair[:eq])
		}
	}
	return names
}

// splitPairs separa "a=\"1\",b=\"2\"" respeitando escapes dentro das aspas.
func splitPairs(labels string) []string {
	var (
		pairs []string
		cur   strings.Builder
		quote bool
		esc   bool
	)
	for _, r := range labels {
		switch {
		case esc:
			esc = false
		case r == '\\':
			esc = true
		case r == '"':
			quote = !quote
		case r == ',' && !quote:
			pairs = append(pairs, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteRune(r)
	}
	pairs = append(pairs, cur.String())
	return pairs
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}
