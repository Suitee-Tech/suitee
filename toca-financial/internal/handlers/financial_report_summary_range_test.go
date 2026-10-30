package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-financial/internal/domain/report"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

// capturingReportRepo registra a janela que o handler repassou ao serviço.
// É o que permite provar a convenção de período sem banco: o defeito do N-02
// era justamente a janela — o último dia chegava truncado em 00:00:00Z.
type capturingReportRepo struct {
	chargeFrom *time.Time
	chargeTo   *time.Time
}

func (r *capturingReportRepo) ChargeTotals(_ context.Context, _ string, from, to *time.Time) ([]report.StatusBucket, error) {
	r.chargeFrom, r.chargeTo = from, to
	return nil, nil
}
func (r *capturingReportRepo) WithdrawalTotals(context.Context, string, *time.Time, *time.Time) ([]report.StatusBucket, error) {
	return nil, nil
}
func (r *capturingReportRepo) DelinquencyTotals(context.Context, string) (report.DelinquencySnapshot, error) {
	return report.DelinquencySnapshot{}, nil
}
func (r *capturingReportRepo) RevenueByType(context.Context, string, time.Time, time.Time) ([]report.CategoryBucket, error) {
	return nil, nil
}
func (r *capturingReportRepo) ExpenseByCategory(context.Context, string, time.Time, time.Time) ([]report.CategoryBucket, error) {
	return nil, nil
}
func (r *capturingReportRepo) CashFlowByDay(context.Context, string, time.Time, time.Time) ([]report.CashFlowEntry, error) {
	return nil, nil
}
func (r *capturingReportRepo) KpisSnapshot(context.Context, string) (report.KPIs, error) {
	return report.KPIs{}, nil
}

func summaryRouter(repo *capturingReportRepo) *gin.Engine {
	handler := NewFinancialReportHandler(service.NewFinancialReportService(repo))
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(pkgcontext.WithTenantID(c.Request.Context(), "tenant-1"))
		c.Next()
	})
	router.GET("/reports/financial/summary", handler.Summary)
	router.GET("/reports/financial/dre", handler.DRE)
	return router
}

func getSummary(t *testing.T, router *gin.Engine, path, query string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, path+"?"+query, nil)
	router.ServeHTTP(recorder, request)
	return recorder
}

// N-02: o Summary usava parseReportDate direto, então um `to` em data-only
// ficava em 00:00:00Z e todo o último dia era descartado — enquanto DRE e
// CashFlow incluíam o dia inteiro para o mesmo intervalo visível.
func TestSummaryIncludesEntireFinalCalendarDay(t *testing.T) {
	repo := &capturingReportRepo{}
	recorder := getSummary(t, summaryRouter(repo), "/reports/financial/summary", "from=2026-09-01&to=2026-09-09")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if repo.chargeTo == nil {
		t.Fatal("o handler não repassou o limite superior")
	}
	loc := FinancialDayLocation()
	lastDayNoon := time.Date(2026, 9, 9, 12, 0, 0, 0, loc)
	if lastDayNoon.After(*repo.chargeTo) {
		t.Fatalf("último dia truncado: to=%s exclui %s", repo.chargeTo.In(loc).Format(time.RFC3339), lastDayNoon.Format(time.RFC3339))
	}
	// O dia seguinte, no mesmo fuso, continua fora.
	nextDayLocal := time.Date(2026, 9, 10, 0, 0, 0, 0, loc)
	if !nextDayLocal.After(*repo.chargeTo) {
		t.Fatalf("dia seguinte deveria estar fora: to=%s", repo.chargeTo.In(loc).Format(time.RFC3339))
	}
}

// O mesmo intervalo precisa significar a mesma coisa em Summary e DRE.
func TestSummaryAndDreAgreeOnRange(t *testing.T) {
	summaryRepo := &capturingReportRepo{}
	getSummary(t, summaryRouter(summaryRepo), "/reports/financial/summary", "from=2026-09-01&to=2026-09-09")

	// DRE usa reportRange, o mesmo resolvedor do Summary: o intervalo visível
	// precisa significar a mesma coisa nos dois.
	_, dreTo, err := reportRange(context.Background(), "tenant-1", "2026-09-01", "2026-09-09")
	if err != nil {
		t.Fatal(err)
	}
	if summaryRepo.chargeTo == nil || !summaryRepo.chargeTo.Equal(dreTo) {
		t.Fatalf("Summary to=%v, DRE to=%v — intervalos divergentes", summaryRepo.chargeTo, dreTo)
	}
}

// Um intervalo invertido é erro de entrada, não 200 com dados arbitrários.
func TestSummaryRejectsInvertedRange(t *testing.T) {
	recorder := getSummary(t, summaryRouter(&capturingReportRepo{}), "/reports/financial/summary", "from=2026-09-10&to=2026-09-09")
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s, want 400", recorder.Code, recorder.Body.String())
	}
}
