package appointment

import (
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/icalendar"
)

// CalendarCopy é a projeção segura de um compromisso da agenda para virar
// cópia de calendário: só o que um evento precisa. Notas internas,
// participantes, telefone, e-mail e descrição livre ficam de fora por decisão
// de privacidade. A serialização fica em pkg/icalendar, compartilhada com o
// portal, que projeta o que o cliente já enxerga.
type CalendarCopy = icalendar.Copy

// publishableStatus lista os status que representam horário marcado. Pedido
// Pendente ainda depende do corretor; Em Andamento/Concluido são passado;
// Cancelado não deve voltar como evento.
func publishableStatus(status Status) bool {
	switch status {
	case StatusAgendado, StatusConfirmado, StatusReagendado:
		return true
	default:
		return false
	}
}

// CalendarCopyFor monta a projeção do compromisso no fuso da imobiliária.
// ok=false quando não é elegível: status fora dos marcos de horário marcado,
// janela incompleta (sem hora final válida) ou horário já passado.
func (a *Appointment) CalendarCopyFor(now time.Time, loc *time.Location) (CalendarCopy, bool) {
	if a == nil || loc == nil || !publishableStatus(a.Status) {
		return CalendarCopy{}, false
	}
	start, err := civilTime(a.Date, a.Time, loc)
	if err != nil {
		return CalendarCopy{}, false
	}
	end, err := civilTime(a.Date, a.EndTime, loc)
	if err != nil || !end.After(start) {
		return CalendarCopy{}, false
	}
	if start.Before(now) {
		return CalendarCopy{}, false
	}
	title := strings.TrimSpace(a.Title)
	if title == "" {
		title = string(a.Type)
	}
	return CalendarCopy{
		UID:      a.ID + "@suitee",
		Title:    title,
		Location: strings.TrimSpace(a.Property),
		Start:    start,
		End:      end,
		TimeZone: loc.String(),
		Stamp:    now.UTC(),
	}, true
}

// civilTime interpreta data (YYYY-MM-DD) e hora (HH:MM) no fuso informado. O
// horário é civil: o mesmo 14:00 vale no fuso da imobiliária, com ou sem
// horário de verão, e o arquivo leva o TZID para o cliente resolver.
func civilTime(date, clock string, loc *time.Location) (time.Time, error) {
	day, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(date), loc)
	if err != nil {
		return time.Time{}, err
	}
	clock = strings.TrimSpace(clock)
	if clock == "" {
		return time.Time{}, errInvalidClock
	}
	hour, minute, err := parseClock(clock)
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, loc), nil
}

type civilError string

func (e civilError) Error() string { return string(e) }

const errInvalidClock = civilError("horário inválido")

func parseClock(clock string) (int, int, error) {
	parts := strings.SplitN(clock, ":", 2)
	if len(parts) != 2 || len(parts[0]) == 0 || len(parts[0]) > 2 || len(parts[1]) != 2 {
		return 0, 0, errInvalidClock
	}
	hour, err := parseTwoDigits(parts[0], 23)
	if err != nil {
		return 0, 0, err
	}
	minute, err := parseTwoDigits(parts[1], 59)
	if err != nil {
		return 0, 0, err
	}
	return hour, minute, nil
}

func parseTwoDigits(value string, max int) (int, error) {
	number := 0
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, errInvalidClock
		}
		number = number*10 + int(r-'0')
	}
	if number > max {
		return 0, errInvalidClock
	}
	return number, nil
}
