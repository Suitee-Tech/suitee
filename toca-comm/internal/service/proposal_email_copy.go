package service

import (
	"fmt"

	"github.com/oginoino/toca/pkg/money"
)

// Copy do e-mail de proposta nova.
//
// O mesmo texto serve para o cliente que já tem conta de portal e para a parte
// que só tem e-mail — antes era a MESMA string `fmt.Sprintf` copiada nos dois
// braços do consumidor, e foi por isso que os dois mostravam "R$ 6000.00": quem
// escrevesse um e-mail novo repetia o `%.2f`.

// proposalSentSubject é o assunto do e-mail de proposta nova.
func proposalSentSubject() string {
	return "Nova proposta no seu negócio"
}

// proposalSentClientBody monta o corpo com o valor da proposta em real
// brasileiro ("R$ 60.000,00").
func proposalSentClientBody(value float64) string {
	return fmt.Sprintf(
		"<p style='margin: 0 0 24px 0; font-size: 16px; color: #4b5563; line-height: 1.6;'>Uma nova proposta foi enviada no valor de <strong>%s</strong>.</p>",
		money.BRL(value),
	)
}

// proposalSentInAppBody é o texto da notificação no painel — o mesmo valor, sem
// a marcação de e-mail.
func proposalSentInAppBody(value float64) string {
	return fmt.Sprintf("Nova proposta enviada no valor de %s", money.BRL(value))
}
