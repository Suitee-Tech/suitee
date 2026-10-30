package deal

import (
	"testing"
	"time"
)

var testSchedule = ProposalFollowUpSchedule{Stage1Days: 3, GapDays: 7, MaxStages: 3}

func sentProposal(at time.Time) *Proposal {
	return &Proposal{ID: "p1", Status: ProposalStatusSent, Value: 1000, CreatedAt: at}
}

// A régua não pode repetir o mesmo estágio: dois ticks seguidos devolvem o
// mesmo estágio (a chave estável do evento colapsa o segundo), e depois do
// mark o estágio avança só quando o intervalo da cadência passa.
func TestProposalFollowUp_StageDoesNotRepeatPerWindow(t *testing.T) {
	created := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	p := sentProposal(created)

	if stage := p.NextFollowUpStage(created.AddDate(0, 0, 2), testSchedule); stage != 0 {
		t.Fatalf("antes do estágio 1 não deve ser devido, got %d", stage)
	}

	due := created.AddDate(0, 0, 3)
	if stage := p.NextFollowUpStage(due, testSchedule); stage != 1 {
		t.Fatalf("esperava estágio 1 em D+3, got %d", stage)
	}
	// Tick seguinte, uma hora depois, ainda sem mark: mesmo estágio 1 —
	// logo, mesmo event_id, logo nenhum segundo e-mail.
	if stage := p.NextFollowUpStage(due.Add(time.Hour), testSchedule); stage != 1 {
		t.Fatalf("esperava estágio 1 no tick seguinte antes do mark, got %d", stage)
	}

	p.MarkFollowUpSent(due)
	// Depois do mark, o estágio 2 só sai quando o gap passa.
	if stage := p.NextFollowUpStage(due.AddDate(0, 0, 6), testSchedule); stage != 0 {
		t.Fatalf("estágio 2 não pode sair antes do gap, got %d", stage)
	}
	if stage := p.NextFollowUpStage(due.AddDate(0, 0, 7), testSchedule); stage != 2 {
		t.Fatalf("esperava estágio 2 em +7d, got %d", stage)
	}
}

func TestProposalFollowUp_StopsWhenAnswered(t *testing.T) {
	created := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	now := created.AddDate(0, 0, 30)

	for _, status := range []ProposalStatus{ProposalStatusAccepted, ProposalStatusRejected, ProposalStatusDraft} {
		p := sentProposal(created)
		p.Status = status
		if stage := p.NextFollowUpStage(now, testSchedule); stage != 0 {
			t.Fatalf("status %s não deve gerar lembrete, got estágio %d", status, stage)
		}
	}
}

func TestProposalFollowUp_CounterOfferChasesBroker(t *testing.T) {
	created := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	p := sentProposal(created)
	p.Status = ProposalStatusCounter

	if got := p.AwaitingFor(); got != ProposalAwaitingBroker {
		t.Fatalf("contraproposta deve aguardar o corretor, got %q", got)
	}
	if stage := p.NextFollowUpStage(created.AddDate(0, 0, 3), testSchedule); stage != 1 {
		t.Fatalf("esperava estágio 1 para contraproposta, got %d", stage)
	}
}

func TestProposalFollowUp_RespectsMaxStages(t *testing.T) {
	created := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	p := sentProposal(created)
	now := created.AddDate(0, 0, 3)

	for i := 0; i < DefaultProposalFollowUpMaxStages; i++ {
		stage := p.NextFollowUpStage(now, testSchedule)
		if stage != i+1 {
			t.Fatalf("estágio %d não saiu no momento esperado (got %d)", i+1, stage)
		}
		p.MarkFollowUpSent(now)
		now = now.AddDate(0, 0, 7)
	}
	// Depois do teto, nenhum lembrete novo — nem com o tempo passando.
	if stage := p.NextFollowUpStage(now.AddDate(0, 0, 365), testSchedule); stage != 0 {
		t.Fatalf("régua deve parar no teto de estágios, got %d", stage)
	}
}

// Proposta velha demais não é perseguida: além de evitar atrito, isso impede
// que a base antiga receba uma rajada de lembretes no primeiro deploy da régua.
func TestProposalFollowUp_StopsAfterMaxAge(t *testing.T) {
	created := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	p := sentProposal(created)
	if stage := p.NextFollowUpStage(created.AddDate(0, 0, 29), testSchedule); stage != 1 {
		t.Fatalf("dentro do teto de idade a régua deve rodar, got %d", stage)
	}
	if stage := p.NextFollowUpStage(created.AddDate(0, 0, 45), testSchedule); stage != 0 {
		t.Fatalf("acima de MaxAgeDays a régua deve parar, got %d", stage)
	}
	if norm := (ProposalFollowUpSchedule{Stage1Days: 3, GapDays: 7, MaxStages: 3}).Norm(); norm.MaxAgeDays != DefaultProposalFollowUpMaxAgeDays {
		t.Fatalf("MaxAgeDays zero deve cair no default, got %d", norm.MaxAgeDays)
	}
}

func TestProposalFollowUp_MaxStagesZeroDisables(t *testing.T) {
	p := sentProposal(time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC))
	off := ProposalFollowUpSchedule{Stage1Days: 0, GapDays: 0, MaxStages: 0}
	if stage := p.NextFollowUpStage(time.Date(2027, 3, 1, 10, 0, 0, 0, time.UTC), off); stage != 0 {
		t.Fatalf("MaxStages=0 desliga a régua, got %d", stage)
	}
	// Norm() mantém o desligamento explícito e preenche os demais defaults.
	if norm := off.Norm(); norm.MaxStages != 0 || norm.Stage1Days != DefaultProposalFollowUpStage1Days || norm.GapDays != DefaultProposalFollowUpGapDays {
		t.Fatalf("Norm inesperada: %+v", norm)
	}
}

func TestProposalFollowUp_NewProposalIsANewGroup(t *testing.T) {
	created := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	first := sentProposal(created)
	first.MarkFollowUpSent(created.AddDate(0, 0, 3))

	// Proposta nova (outro id, outra âncora) começa a contagem do zero — o
	// grupo é por proposta, não pela conversa.
	second := &Proposal{ID: "p2", Status: ProposalStatusSent, CreatedAt: created.AddDate(0, 0, 20)}
	if stage := second.NextFollowUpStage(created.AddDate(0, 0, 22), testSchedule); stage != 0 {
		t.Fatalf("proposta nova não deve lembrar antes de D+3, got %d", stage)
	}
	if stage := second.NextFollowUpStage(created.AddDate(0, 0, 23), testSchedule); stage != 1 {
		t.Fatalf("proposta nova deve lembrar em D+3, got %d", stage)
	}
}

func TestProposalFollowUp_DaysPendingNeverNegative(t *testing.T) {
	created := time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC)
	p := sentProposal(created)
	if got := p.DaysPending(created.Add(-90 * time.Minute)); got != 0 {
		t.Fatalf("DaysPending não pode ser negativo, got %d", got)
	}
	if got := p.DaysPending(created.AddDate(0, 0, 9).Add(6 * time.Hour)); got != 9 {
		t.Fatalf("DaysPending esperado 9, got %d", got)
	}
}

func TestProposalByID(t *testing.T) {
	d := &Deal{Proposals: []Proposal{{ID: "a"}, {ID: "b"}}}
	if got := d.ProposalByID("b"); got == nil || got.ID != "b" {
		t.Fatalf("ProposalByID devolveu %+v", got)
	}
	if got := d.ProposalByID("z"); got != nil {
		t.Fatalf("ProposalByID de id inexistente deve ser nil, got %+v", got)
	}
}
