package broker

import (
	"time"

	"github.com/google/uuid"
)

type Broker struct {
	ID     string `bson:"_id"`
	Name   string `bson:"name"`
	Role   string `bson:"role"`
	Email  string `bson:"email,omitempty"`
	Avatar string `bson:"avatar,omitempty"`
	Active bool   `bson:"active"`
	// MaxActiveLeads define a capacidade máxima de leads "ativos" atribuídos ao corretor.
	// 0 = sem limite (recomendado para não bloquear distribuição por engano).
	MaxActiveLeads int      `bson:"max_active_leads,omitempty"`
	Specialties    []string `bson:"specialties,omitempty"`
	Regions        []string `bson:"regions,omitempty"`
	Weight         float64  `bson:"weight,omitempty"`
	IsPriority     bool     `bson:"is_priority,omitempty"`
	Paused         bool     `bson:"paused,omitempty" json:"paused,omitempty"`
	// PausedUntil habilita auto-unpause: se setado, o broker é tratado como
	// ativo quando time.Now() ultrapassa o valor. SLA worker preenche isto
	// no auto-pause (Wave P1.7) para evitar broker preso fora da roleta
	// até intervenção manual.
	PausedUntil   *time.Time     `bson:"paused_until,omitempty" json:"pausedUntil,omitempty"`
	BusinessHours *BusinessHours `bson:"business_hours,omitempty" json:"businessHours,omitempty"`
	TenantID      string         `bson:"tenant_id"`
	CreatedAt     time.Time      `bson:"created_at"`
	UpdatedAt     time.Time      `bson:"updated_at"`
}

type BrokerFilter struct {
	Search      string
	Specialties []string
	Regions     []string
	Active      *bool
	Paused      *bool
	Role        string
	IsPriority  *bool
	Limit       int64
	Offset      int64
}

// DefaultBrokerPauseCooldown é o tempo padrão de auto-unpause quando o
// SLA tier não especifica um cooldown próprio. 4h cobre um turno de
// plantão sem precisar de admin manual; ajuste por tenant via SLATier.
const DefaultBrokerPauseCooldown = 4 * time.Hour

// IsCurrentlyPaused reporta se o broker está pausado neste instante,
// respeitando PausedUntil. Um broker com Paused=true e PausedUntil
// no passado é tratado como ativo (auto-unpause expirado). Quando
// PausedUntil é nil, mantém a semântica antiga (pausa indefinida).
func (b *Broker) IsCurrentlyPaused(now time.Time) bool {
	if b == nil || !b.Paused {
		return false
	}
	if b.PausedUntil != nil && now.After(*b.PausedUntil) {
		return false
	}
	return true
}

func New(name, role, email, tenantID string) *Broker {
	now := time.Now()
	return &Broker{
		ID:        uuid.New().String(),
		Name:      name,
		Role:      role,
		Email:     email,
		Active:    true,
		TenantID:  tenantID,
		CreatedAt: now,
		UpdatedAt: now,
	}
}
