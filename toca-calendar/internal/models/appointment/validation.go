package appointment

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

var ErrInvalid = errors.New("dados inválidos para o compromisso")
var ErrConflict = errors.New("scheduling conflict: agent already has an appointment at this time")

// Validate permits historical administrative appointments. Client-facing
// services additionally require a future slot before calling the calendar.
// Unchanged legacy enums may be maintained, but cannot be introduced anew.
func (a *Appointment) Validate(previous *Appointment) error {
	if a == nil {
		return ErrInvalid
	}
	date, err := time.Parse("2006-01-02", a.Date)
	if err != nil || date.Format("2006-01-02") != a.Date {
		return fmt.Errorf("%w: informe a data no formato AAAA-MM-DD", ErrInvalid)
	}
	start, err := time.Parse("15:04", a.Time)
	if err != nil || start.Format("15:04") != a.Time {
		return fmt.Errorf("%w: informe um horário inicial válido", ErrInvalid)
	}
	end, err := time.Parse("15:04", a.EndTime)
	if err != nil || end.Format("15:04") != a.EndTime || !end.After(start) {
		return fmt.Errorf("%w: o horário final deve ser posterior ao inicial no mesmo dia", ErrInvalid)
	}
	if (previous == nil || a.Type != previous.Type) && !slices.Contains([]Type{TypeVisita, TypeReuniao, "Reunião", TypeVistoria, TypeAssinatura, TypeEntrega, "Ligacao", "Lembrete", "Outro"}, a.Type) {
		return fmt.Errorf("%w: tipo desconhecido", ErrInvalid)
	}
	if (previous == nil || a.Status != previous.Status) && !slices.Contains([]Status{StatusPendente, StatusReagendado, StatusAgendado, StatusConfirmado, StatusEmAndamento, StatusConcluido, "Concluído", StatusCancelado}, a.Status) {
		return fmt.Errorf("%w: status desconhecido", ErrInvalid)
	}
	if (previous == nil || a.Priority != previous.Priority) && !slices.Contains([]Priority{PriorityAlta, PriorityMedia, PriorityBaixa}, a.Priority) {
		return fmt.Errorf("%w: prioridade desconhecida", ErrInvalid)
	}
	if (previous == nil || a.Visibility != previous.Visibility) && !slices.Contains([]Visibility{"", VisibilityTeam, VisibilityPrivate}, a.Visibility) {
		return fmt.Errorf("%w: visibilidade desconhecida", ErrInvalid)
	}
	return nil
}

// ErrConcurrentModification indicates an edit based on an obsolete revision.
var ErrConcurrentModification = errors.New("A agenda foi alterada. Atualize a página antes de tentar novamente.")
