package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
	appcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
)

func TestFinancialReportChartRequiresTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/reports/financial/chart-of-accounts", nil)
	NewFinancialReportHandler(nil).ChartOfAccounts(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d; want 403", w.Code)
	}
}

func TestFinancialReportChartReturnsCanonicalCatalogueWithoutDatabase(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/reports/financial/chart-of-accounts", nil)
	c.Request = c.Request.WithContext(appcontext.WithTenantID(c.Request.Context(), "tenant-a"))
	NewFinancialReportHandler(nil).ChartOfAccounts(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200: %s", w.Code, w.Body.String())
	}
	var body struct {
		Data journal.ChartOfAccounts `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(body.Data, journal.GetChartOfAccounts()) {
		t.Fatalf("response differs from canonical catalogue: %+v", body.Data)
	}
}
