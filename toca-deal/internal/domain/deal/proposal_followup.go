package deal

import "time"

// Régua de follow-up de proposta (plano beta §F6).
//
// O problema que esta regra resolve é o mesmo do lembrete de chat: um lembrete
// cuja chave de idempotência é derivada do TEMPO (hora/dia da varredura) volta
// a sair a cada janela enquanto a pendência continua, virando spam. Aqui a
// chave é o GRUPO — (proposta, estágio) — e o estágio só avança depois que o
// anterior foi disparado e o intervalo da cadência passou. Enquanto a proposta
// continuar aguardando a mesma resposta, nenhum lembrete novo sai.

const (
	// DefaultProposalFollowUpStage1Days é quanto tempo depois do envio da
	// proposta o primeiro lembrete sai.
	DefaultProposalFollowUpStage1Days = 3
	// DefaultProposalFollowUpGapDays é o intervalo entre lembretes seguintes.
	DefaultProposalFollowUpGapDays = 7
	// DefaultProposalFollowUpMaxStages limita o total de lembretes por
	// proposta — o teto existe para a régua não virar perseguição.
	DefaultProposalFollowUpMaxStages = 3
	// DefaultProposalFollowUpMaxAgeDays é a idade máxima da pendência para a
	// régua continuar: passou disso, a negociação esfriou e cobrar por e-mail
	// só gera atrito (e evita que uma base antiga receba lembretes em massa
	// no primeiro deploy da régua).
	DefaultProposalFollowUpMaxAgeDays = 30
)

// ProposalAwaitingFor diz quem está com a resposta da proposta.
type ProposalAwaitingFor string

const (
	// ProposalAwaitingClient: proposta enviada pelo corretor aguardando o
	// cliente (buyer/tenant).
	ProposalAwaitingClient ProposalAwaitingFor = "client"
	// ProposalAwaitingBroker: contraproposta do cliente aguardando o corretor.
	ProposalAwaitingBroker ProposalAwaitingFor = "broker"
)

// ProposalFollowUpSchedule é a cadência da régua. Os campos vêm de env vars
// (ver main.go) e passam por Norm() para nunca valerem zero/negativo.
type ProposalFollowUpSchedule struct {
	Stage1Days int
	GapDays    int
	MaxStages  int
	// MaxAgeDays é o teto de idade da pendência (dias desde o envio da
	// proposta). <= 0 cai no default.
	MaxAgeDays int
}

// Norm devolve a cadência com defaults para valores inválidos. MaxStages <= 0
// significa "régua desligada" (nenhum lembrete sai), preservado de propósito:
// é o kill-switch explícito.
func (s ProposalFollowUpSchedule) Norm() ProposalFollowUpSchedule {
	out := ProposalFollowUpSchedule{
		Stage1Days: s.Stage1Days,
		GapDays:    s.GapDays,
		MaxStages:  s.MaxStages,
		MaxAgeDays: s.MaxAgeDays,
	}
	if out.Stage1Days <= 0 {
		out.Stage1Days = DefaultProposalFollowUpStage1Days
	}
	if out.GapDays <= 0 {
		out.GapDays = DefaultProposalFollowUpGapDays
	}
	if out.MaxAgeDays <= 0 {
		out.MaxAgeDays = DefaultProposalFollowUpMaxAgeDays
	}
	return out
}

// AwaitingFor mapeia o status da proposta para quem precisa responder. Status
// terminal (ACCEPTED/REJECTED/DRAFT) não tem destinatário — a régua para.
func (p *Proposal) AwaitingFor() ProposalAwaitingFor {
	if p == nil {
		return ""
	}
	switch p.Status {
	case ProposalStatusSent:
		return ProposalAwaitingClient
	case ProposalStatusCounter:
		return ProposalAwaitingBroker
	default:
		return ""
	}
}

// FollowUpAnchor devolve o instante a partir do qual a cadência é contada: o
// último lembrete disparado ou, se ainda não houve, o envio da proposta.
func (p *Proposal) FollowUpAnchor() time.Time {
	if p == nil {
		return time.Time{}
	}
	if p.FollowUpLastAt != nil && !p.FollowUpLastAt.IsZero() {
		return *p.FollowUpLastAt
	}
	return p.CreatedAt
}

// NextFollowUpStage devolve o estágio do lembrete devido em `now` (0 = nada
// devido). Só há um estágio devido por vez, e só quando a proposta continua
// aguardando resposta. A regra é determinística em (status, FollowUps,
// FollowUpLastAt): rodar a varredura duas vezes seguidas devolve o mesmo
// estágio, e é a chave estável do evento que garante o envio único.
func (p *Proposal) NextFollowUpStage(now time.Time, sched ProposalFollowUpSchedule) int {
	if p == nil || p.AwaitingFor() == "" {
		return 0
	}
	norm := sched.Norm()
	if norm.MaxStages <= 0 {
		return 0
	}
	if p.FollowUps >= norm.MaxStages {
		return 0
	}
	if p.DaysPending(now) > norm.MaxAgeDays {
		// Pendência velha demais: a negociação esfriou e a régua para.
		return 0
	}
	anchor := p.FollowUpAnchor()
	if anchor.IsZero() {
		return 0
	}
	wait := norm.GapDays
	if p.FollowUps == 0 {
		wait = norm.Stage1Days
	}
	if now.Before(anchor.AddDate(0, 0, wait)) {
		return 0
	}
	return p.FollowUps + 1
}

// DaysPending devolve há quantos dias a proposta aguarda resposta, contados do
// envio (nunca negativo).
func (p *Proposal) DaysPending(now time.Time) int {
	if p == nil || p.CreatedAt.IsZero() {
		return 0
	}
	days := int(now.Sub(p.CreatedAt).Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
}

// MarkFollowUpSent registra o disparo do lembrete, ancorando a próxima janela.
// Chamado pelo motor DEPOIS de publicar o evento: se o processo cair entre o
// publish e o mark, o próximo tick republica o MESMO event_id e o dedup do
// consumidor colapsa — nunca vira dois e-mails.
func (p *Proposal) MarkFollowUpSent(at time.Time) {
	if p == nil {
		return
	}
	p.FollowUps++
	stamp := at
	p.FollowUpLastAt = &stamp
}

// ProposalByID devolve a proposta pelo id (nil quando não existe).
func (d *Deal) ProposalByID(proposalID string) *Proposal {
	if d == nil {
		return nil
	}
	for i := range d.Proposals {
		if d.Proposals[i].ID == proposalID {
			return &d.Proposals[i]
		}
	}
	return nil
}
