package service

import "fmt"

// Copy dos e-mails transacionais de agenda (beta F2 — docs/features/
// comunicacao/notificacoes-email-beta.md). Fraseado neutro ("Compromisso")
// para não depender de gênero/plural dos tipos (Visita/Reunião/Vistoria…).

// appointmentTypeNoun devolve o substantivo pt-BR do tipo de compromisso;
// "" para tipos desconhecidos (caller cai para "Compromisso").
func appointmentTypeNoun(t string) string {
	switch t {
	case "Visita":
		return "Visita"
	case "Reuniao":
		return "Reunião"
	case "Vistoria":
		return "Vistoria"
	case "Assinatura":
		return "Assinatura"
	case "Entrega":
		return "Entrega"
	default:
		return ""
	}
}

// appointmentDisplayName devolve o nome do compromisso para copy: o título
// quando existe, senão o substantivo do tipo, senão "Compromisso".
func appointmentDisplayName(title, apptType string) string {
	if title != "" {
		return title
	}
	if noun := appointmentTypeNoun(apptType); noun != "" {
		return noun
	}
	return "Compromisso"
}

// appointmentWhen formata data/hora do compromisso: "2026-10-10 às 14:00",
// só data quando não há hora, "" quando não há nada.
func appointmentWhen(date, timeVal string) string {
	if date == "" {
		return ""
	}
	if timeVal == "" {
		return date
	}
	return date + " às " + timeVal
}

// appointmentStatusNeedsEmail define o gate de status do beta: só os marcos
// de confirmação, reagendamento e cancelamento viram e-mail; os demais
// (Pendente, Agendado, Em Andamento, Concluido) permanecem apenas in-app.
func appointmentStatusNeedsEmail(status string) bool {
	switch status {
	case "Confirmado", "Reagendado", "Cancelado":
		return true
	default:
		return false
	}
}

// appointmentClientSubjectCreated distingue o pedido ainda pendente do
// horário já marcado: o cliente pediu um horário pelo portal e quem confirma é
// o corretor, então a comunicação não pode anunciar como agendado.
func appointmentClientSubjectCreated(title, apptType, status string) string {
	if appointmentPendingStatus(status) {
		return "Solicitação de horário recebida: " + appointmentDisplayName(title, apptType)
	}
	return "Compromisso agendado: " + appointmentDisplayName(title, apptType)
}

func appointmentClientBodyCreated(title, apptType, when, status string) string {
	paragraph := func(text string) string {
		return fmt.Sprintf("<p style='margin: 0 0 24px 0; font-size: 16px; color: #4b5563; line-height: 1.6;'>%s</p>", text)
	}
	whenLine := ""
	if when != "" {
		whenLine = fmt.Sprintf(" para <strong>%s</strong>", when)
	}
	if appointmentPendingStatus(status) {
		return paragraph(fmt.Sprintf("Recebemos a sua solicitação de horário%s.", whenLine)) +
			paragraph("O corretor confirma a disponibilidade e você recebe um novo aviso assim que o horário estiver marcado.")
	}
	noun := appointmentTypeNoun(apptType)
	what := "Um novo compromisso foi agendado"
	if noun != "" {
		what = fmt.Sprintf("Uma nova <strong>%s</strong> foi agendada", noun)
	}
	return paragraph(what+whenLine+".") + paragraph(appointmentDisplayName(title, apptType))
}

// appointmentPendingStatus cobre o pedido do portal aguardando confirmação.
func appointmentPendingStatus(status string) bool { return status == "Pendente" }

// appointmentCalendarCTA decide o convite do e-mail de agenda: com arquivo de
// calendário o botão principal é o download e o portal vira link secundário;
// sem arquivo (pendente, cancelado ou assinador desligado) o portal continua
// sendo o botão.
func appointmentCalendarCTA(calendarURL, portalURL string) (btnText, btnURL, secondaryText, secondaryURL string) {
	if calendarURL == "" {
		return "Ver no portal", portalURL, "", ""
	}
	return "Adicionar ao calendário", calendarURL, "Ver no portal", portalURL
}

func appointmentClientSubjectChanged(status, title string) string {
	name := appointmentDisplayName(title, "")
	switch status {
	case "Confirmado":
		return "Compromisso confirmado: " + name
	case "Reagendado":
		return "Compromisso reagendado: " + name
	case "Cancelado":
		return "Compromisso cancelado: " + name
	default:
		return "Atualização do seu compromisso: " + name
	}
}

func appointmentClientBodyChanged(status, title, when string) string {
	name := appointmentDisplayName(title, "")
	whenLine := ""
	if when != "" {
		whenLine = fmt.Sprintf(" (<strong>%s</strong>)", when)
	}
	var line string
	switch status {
	case "Confirmado":
		line = fmt.Sprintf("Seu compromisso <strong>%s</strong> foi confirmado%s.", name, whenLine)
	case "Reagendado":
		line = fmt.Sprintf("Seu compromisso <strong>%s</strong> foi reagendado%s.", name, whenLine)
	case "Cancelado":
		line = fmt.Sprintf("Seu compromisso <strong>%s</strong>%s foi cancelado.", name, whenLine)
	default:
		line = fmt.Sprintf("Seu compromisso <strong>%s</strong> foi atualizado%s.", name, whenLine)
	}
	return fmt.Sprintf("<p style='margin: 0 0 24px 0; font-size: 16px; color: #4b5563; line-height: 1.6;'>%s</p>", line)
}

func appointmentOperatorSubjectChanged(status, title string) string {
	name := appointmentDisplayName(title, "")
	switch status {
	case "Confirmado":
		return "Compromisso confirmado: " + name
	case "Reagendado":
		return "Compromisso reagendado: " + name
	case "Cancelado":
		return "Compromisso cancelado: " + name
	default:
		return "Atualização de compromisso: " + name
	}
}

func appointmentOperatorBodyChanged(status, title, when string) string {
	name := appointmentDisplayName(title, "")
	whenLine := ""
	if when != "" {
		whenLine = fmt.Sprintf(" para <strong>%s</strong>", when)
	}
	var line string
	switch status {
	case "Confirmado":
		line = fmt.Sprintf("O compromisso <strong>%s</strong> foi confirmado%s.", name, whenLine)
	case "Reagendado":
		line = fmt.Sprintf("O compromisso <strong>%s</strong> foi reagendado%s.", name, whenLine)
	case "Cancelado":
		line = fmt.Sprintf("O compromisso <strong>%s</strong> foi cancelado.", name)
	default:
		line = fmt.Sprintf("O compromisso <strong>%s</strong> foi atualizado%s.", name, whenLine)
	}
	return fmt.Sprintf("<p style='margin: 0 0 24px 0; font-size: 16px; color: #4b5563; line-height: 1.6;'>%s</p>", line)
}
