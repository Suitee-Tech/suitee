package handlers

import (
	"errors"
	"fmt"
	"testing"

	ps "github.com/oginoino/toca/toca-financial/internal/domain/platform_subscription"
)

// Em produção (2026-10-08) o operador em trial tentou assinar e recebeu só
// "A operação não está disponível no estado atual…": o motivo gravado pelo
// serviço era descartado no mapeamento do erro.
func TestMensagemDoConflitoUsaOMotivoDoServico(t *testing.T) {
	casos := []struct {
		nome    string
		err     error
		quer    string
		naoQuer string
	}{
		{
			nome: "motivo específico",
			err:  fmt.Errorf("%w: o checkout do plano Pro já foi pago; aguarde a confirmação para trocar de plano", ps.ErrConflict),
			quer: "O checkout do plano Pro já foi pago; aguarde a confirmação para trocar de plano.",
		},
		{
			nome: "uso acima dos limites",
			err:  fmt.Errorf("%w: uso acima dos limites do plano", ps.ErrConflict),
			quer: "Uso acima dos limites do plano.",
		},
		{
			nome:    "sentinela sem motivo mantém o texto genérico",
			err:     ps.ErrConflict,
			naoQuer: "",
			quer:    "",
		},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			detalhe := subscriptionDetail(caso.err, ps.ErrConflict)
			if caso.quer == "" {
				if detalhe != "" {
					t.Fatalf("esperava motivo vazio, veio %q", detalhe)
				}
				return
			}
			frase := subscriptionSentence(detalhe)
			if frase != caso.quer {
				t.Fatalf("mensagem %q, esperado %q", frase, caso.quer)
			}
		})
	}
}

func TestSubscriptionDetailIgnoraErroDeOutroTipo(t *testing.T) {
	if detalhe := subscriptionDetail(errors.New("erro qualquer"), ps.ErrConflict); detalhe != "" {
		t.Fatalf("erro de outro tipo não pode virar motivo: %q", detalhe)
	}
}
