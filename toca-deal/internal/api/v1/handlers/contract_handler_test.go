package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/toca-deal/internal/domain/contract"
)

func TestRespondContractCreateError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "unauthorized", err: errors.New("unauthorized"), wantStatus: http.StatusUnauthorized},
		{name: "missing deal", err: errors.New("deal not found"), wantStatus: http.StatusNotFound},
		{
			name:       "invalid signatory role",
			err:        fmt.Errorf("%w 'conjuge'", contract.ErrInvalidSignatoryRole),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid period",
			err:        fmt.Errorf("%w: end_date must be >= start_date", contract.ErrInvalidContractPeriod),
			wantStatus: http.StatusBadRequest,
		},
		{name: "repository failure", err: errors.New("database unavailable"), wantStatus: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			respondContractCreateError(ctx, tt.err)

			if recorder.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
		})
	}
}
