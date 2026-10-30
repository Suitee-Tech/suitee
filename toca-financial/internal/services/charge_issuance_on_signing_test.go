package services_test

import (
	"context"
	"sync"
	"testing"

	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/rental_charge_config"
	service "github.com/oginoino/toca/toca-financial/internal/services"
	"github.com/stretchr/testify/assert"
)

// ── Fakes mínimos para exercitar a emissão na assinatura ──────────────
//
// Ficam neste pacote porque o input do handler é montado pelo builder daqui.
// São pequenos: as duas interfaces somam 8 métodos.

type assinaturaConfigRepo struct {
	mu  sync.Mutex
	cfg *rental_charge_config.RentalChargeConfig
}

func (r *assinaturaConfigRepo) Create(_ context.Context, cfg *rental_charge_config.RentalChargeConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cfg = cfg
	return nil
}

func (r *assinaturaConfigRepo) GetByID(_ context.Context, tenantID, id string) (*rental_charge_config.RentalChargeConfig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cfg != nil && r.cfg.TenantID == tenantID && r.cfg.ID == id {
		return r.cfg, nil
	}
	return nil, nil
}

func (r *assinaturaConfigRepo) GetByContractID(_ context.Context, tenantID, contractID string) (*rental_charge_config.RentalChargeConfig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cfg != nil && r.cfg.TenantID == tenantID && r.cfg.ContractID == contractID {
		return r.cfg, nil
	}
	return nil, nil
}

func (r *assinaturaConfigRepo) Update(_ context.Context, cfg *rental_charge_config.RentalChargeConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cfg = cfg
	return nil
}

func (r *assinaturaConfigRepo) Delete(context.Context, string, string) error { return nil }

// assinaturaChargeRepo reproduz o comportamento do repositório real: a
// competência é a chave de idempotência, então uma segunda passada não cria
// linha nova para a mesma competência.
type assinaturaChargeRepo struct {
	mu       sync.Mutex
	created  []*charge.Charge
	porChave map[string]bool
}

func newAssinaturaChargeRepo() *assinaturaChargeRepo {
	return &assinaturaChargeRepo{porChave: map[string]bool{}}
}

func (r *assinaturaChargeRepo) Create(_ context.Context, c *charge.Charge) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	chave := c.TenantID + "|" + c.ContractID + "|" + c.ReferenceMonth
	if r.porChave[chave] {
		return nil
	}
	r.porChave[chave] = true
	r.created = append(r.created, c)
	return nil
}

func (r *assinaturaChargeRepo) Update(context.Context, *charge.Charge) error { return nil }

func (r *assinaturaChargeRepo) List(context.Context, string, string, string, string, int64, int64) ([]*charge.Charge, int64, error) {
	return nil, 0, nil
}

func (r *assinaturaChargeRepo) total() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.created)
}

func novoHandlerDeAssinatura(chargeRepo *assinaturaChargeRepo) *service.ChargeIssuanceHandler {
	rentalConfigService := service.NewRentalChargeConfigService(&assinaturaConfigRepo{}, chargeRepo, nil)
	return service.NewChargeIssuanceHandler(service.NewChargeService(&mockChargeRepo{}, nil), rentalConfigService, nil, nil)
}

// A assinatura de um contrato de locação precisa EMITIR as cobranças.
//
// Antes, o handler provisionava apenas o cronograma (`rental_charge_config`) e a
// emissão era etapa manual na tela de Locação ("Emitir cobranças previstas").
// O resultado apareceu em produção: um contrato de locação assinado com config
// criada e ZERO cobranças — um contrato que não cobra, sem nada na tela de
// contrato sinalizando. O contrato original do mesmo acordo tinha 31 cobranças;
// o aditivo assinado, nenhuma.
func TestChargeIssuanceHandler_RENT_EmitsChargesOnSigning(t *testing.T) {
	chargeRepo := newAssinaturaChargeRepo()
	h := novoHandlerDeAssinatura(chargeRepo)

	assert.NoError(t, h.Handle(context.Background(), newRentInput(t)))
	assert.NotEmpty(t, chargeRepo.created,
		"a assinatura de locação precisa emitir as cobranças, e não só provisionar o cronograma")
}

// A reentrega do evento não pode duplicar cobranças: cada competência é única.
// Sem isso, ativar a emissão na assinatura seria pior que o problema original.
func TestChargeIssuanceHandler_RENT_RedeliveryDoesNotDuplicateCharges(t *testing.T) {
	chargeRepo := newAssinaturaChargeRepo()
	h := novoHandlerDeAssinatura(chargeRepo)
	input := newRentInput(t)

	assert.NoError(t, h.Handle(context.Background(), input))
	primeira := chargeRepo.total()
	assert.NotZero(t, primeira)

	assert.NoError(t, h.Handle(context.Background(), input))
	assert.Equal(t, primeira, chargeRepo.total(),
		"reentrega do contract.signed não pode criar cobranças duplicadas")
}
