package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/clients"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

type commissionLifecycleConflictRepo struct {
	service.CommissionRepository
	item     *commission.Commission
	forceCAS bool
}

type commissionContractLookupHandlerStub struct {
	contract *clients.ContractDTO
}

func (s *commissionContractLookupHandlerStub) GetContract(_ context.Context, _, _ string) (*clients.ContractDTO, error) {
	return s.contract, nil
}

func (r *commissionLifecycleConflictRepo) GetByID(_ context.Context, tenantID, id string) (*commission.Commission, error) {
	if r.item == nil || r.item.TenantID != tenantID || r.item.ID != id {
		return nil, nil
	}
	clone := *r.item
	clone.Deductions = append([]commission.Deduction(nil), r.item.Deductions...)
	return &clone, nil
}

func (r *commissionLifecycleConflictRepo) Update(_ context.Context, updated *commission.Commission) error {
	clone := *updated
	clone.Deductions = append([]commission.Deduction(nil), updated.Deductions...)
	r.item = &clone
	return nil
}

func (r *commissionLifecycleConflictRepo) UpdateIfStatus(_ context.Context, updated *commission.Commission, _ commission.Status, _ time.Time) error {
	if r.forceCAS {
		return commission.ErrConcurrentTransition
	}
	return r.Update(context.Background(), updated)
}

func TestCommissionLifecycleConflictsMapToHTTP409(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name         string
		path         string
		wire         func(*gin.Engine, *CommissionHandler)
		linked       bool
		funding      bool
		legacyRental bool
		forceCAS     bool
	}{
		{name: "linked approve", path: "/commissions/commission-conflict/approve", linked: true, wire: func(router *gin.Engine, handler *CommissionHandler) {
			router.POST("/commissions/:id/approve", handler.ApproveCommission)
		}},
		{name: "linked pay", path: "/commissions/commission-conflict/pay", linked: true, wire: func(router *gin.Engine, handler *CommissionHandler) {
			router.POST("/commissions/:id/pay", handler.MarkPaid)
		}},
		{name: "linked cancel", path: "/commissions/commission-conflict/cancel", linked: true, wire: func(router *gin.Engine, handler *CommissionHandler) {
			router.POST("/commissions/:id/cancel", handler.CancelCommission)
		}},
		{name: "standalone CAS", path: "/commissions/commission-conflict/approve", forceCAS: true, wire: func(router *gin.Engine, handler *CommissionHandler) {
			router.POST("/commissions/:id/approve", handler.ApproveCommission)
		}},
		{name: "rental funding incomplete", path: "/commissions/commission-conflict/approve", funding: true, wire: func(router *gin.Engine, handler *CommissionHandler) {
			router.POST("/commissions/:id/approve", handler.ApproveCommission)
		}},
		{name: "legacy rental reconciliation", path: "/commissions/commission-conflict/approve", legacyRental: true, wire: func(router *gin.Engine, handler *CommissionHandler) {
			router.POST("/commissions/:id/approve", handler.ApproveCommission)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := commission.New("deal-conflict", "broker-conflict", 100, "tenant-conflict")
			item.ID = "commission-conflict"
			if tt.linked {
				item.PayoutOrderID = "payout-owner"
			}
			if tt.funding {
				item.FundingRequiredCents = 10000
			}
			if tt.legacyRental {
				item.ContractID = "contract-legacy-rent"
			}
			repo := &commissionLifecycleConflictRepo{item: item, forceCAS: tt.forceCAS}
			commissionService := service.NewCommissionService(repo)
			if tt.legacyRental {
				commissionService.WithContractLookup(&commissionContractLookupHandlerStub{contract: &clients.ContractDTO{
					ID: item.ContractID, TenantID: item.TenantID, Type: "RENT",
				}})
			}
			handler := NewCommissionHandler(commissionService)
			router := gin.New()
			router.Use(func(c *gin.Context) {
				ctx := pkgcontext.WithTenantID(c.Request.Context(), item.TenantID)
				c.Request = c.Request.WithContext(ctx)
				c.Next()
			})
			tt.wire(router, handler)

			responseRecorder := httptest.NewRecorder()
			// /pay é baixa manual e exige comprovante pela política do tenant;
			// sem ele o guard responde 400 e o conflito testado não é alcançado.
			request := httptest.NewRequest(http.MethodPost, tt.path,
				strings.NewReader(`{"proof_url":"https://files.example/comissao.pdf"}`))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(responseRecorder, request)

			if responseRecorder.Code != http.StatusConflict {
				t.Fatalf("status=%d body=%s, want 409", responseRecorder.Code, responseRecorder.Body.String())
			}
		})
	}
}
