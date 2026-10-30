package icalendar

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Serialização iCalendar (RFC 5545) da cópia de um compromisso. O arquivo é
// uma cópia pontual: METHOD:PUBLISH, sem ATTENDEE e sem RSVP. Quem importa não
// recebe atualizações automáticas — o UID é estável para que um novo arquivo
// substitua o anterior quando a pessoa importar de novo.

const (
	// Folda em 75 octetos, como o RFC 5545 pede (excluindo o CRLF).
	maxLineOctets = 75
	producerID    = "-//Suitee//Agenda//PT-BR"
)

// Copy é a projeção segura de um compromisso: só o que um evento de calendário
// precisa. Cada serviço monta a sua — a agenda tem o compromisso completo, o
// portal projeta o que o cliente já enxerga — e usa este mesmo serializador.
type Copy struct {
	UID      string
	Title    string
	Location string
	Start    time.Time
	End      time.Time
	TimeZone string
	Stamp    time.Time
}

// Render devolve o arquivo .ics pronto para download.
func Render(copy Copy) ([]byte, error) {
	if copy.UID == "" || copy.TimeZone == "" {
		return nil, fmt.Errorf("cópia de calendário incompleta")
	}
	if !copy.End.After(copy.Start) {
		return nil, fmt.Errorf("janela do compromisso inválida")
	}
	loc, err := time.LoadLocation(copy.TimeZone)
	if err != nil {
		return nil, fmt.Errorf("fuso desconhecido: %w", err)
	}
	stamp := copy.Stamp
	if stamp.IsZero() {
		stamp = time.Now()
	}

	lines := []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:" + producerID,
		"CALSCALE:GREGORIAN",
		"METHOD:PUBLISH",
	}
	lines = append(lines, vtimezoneLines(copy.TimeZone, loc, copy.Start)...)
	lines = append(lines,
		"BEGIN:VEVENT",
		"UID:"+escapeText(copy.UID),
		"DTSTAMP:"+stamp.UTC().Format("20060102T150405Z"),
		"DTSTART;TZID="+copy.TimeZone+":"+copy.Start.Format("20060102T150405"),
		"DTEND;TZID="+copy.TimeZone+":"+copy.End.Format("20060102T150405"),
		"SUMMARY:"+escapeText(copy.Title),
	)
	if copy.Location != "" {
		lines = append(lines, "LOCATION:"+escapeText(copy.Location))
	}
	lines = append(lines,
		"STATUS:CONFIRMED",
		"TRANSP:OPAQUE",
		"SEQUENCE:0",
		"END:VEVENT",
		"END:VCALENDAR",
	)

	var out strings.Builder
	for _, line := range lines {
		for _, folded := range foldLine(line) {
			out.WriteString(folded)
			out.WriteString("\r\n")
		}
	}
	return []byte(out.String()), nil
}

// escapeText aplica as regras de texto do RFC 5545: barra invertida, ponto e
// vírgula, vírgula e quebras de linha.
var textEscaper = strings.NewReplacer(
	`\`, `\\`,
	";", `\;`,
	",", `\,`,
	"\r\n", `\n`,
	"\n", `\n`,
	"\r", `\n`,
)

func escapeText(value string) string { return textEscaper.Replace(value) }

// foldLine quebra linhas longas em octetos, sem partir um caractere UTF-8.
func foldLine(line string) []string {
	if len(line) <= maxLineOctets {
		return []string{line}
	}
	folded := []string{}
	var current strings.Builder
	limit := maxLineOctets
	for _, r := range line {
		size := len(string(r))
		if current.Len()+size > limit {
			folded = append(folded, current.String())
			current.Reset()
			current.WriteString(" ")
			limit = maxLineOctets
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		folded = append(folded, current.String())
	}
	return folded
}

// vtimezoneLines descreve o fuso do compromisso. O RFC 5545 pede as regras do
// fuso no próprio arquivo: as transições observadas na janela viram componentes
// STANDARD/DAYLIGHT, com o horário local no offset de origem. Fuso sem horário
// de verão (o caso do Brasil desde 2019) rende um único STANDARD.
func vtimezoneLines(tzid string, loc *time.Location, around time.Time) []string {
	observances := zoneObservances(loc, around.AddDate(-1, 0, 0), around.AddDate(1, 0, 0))
	lines := []string{"BEGIN:VTIMEZONE", "TZID:" + tzid, "X-LIC-LOCATION:" + tzid}
	for _, ob := range observances {
		kind := "STANDARD"
		if ob.dst {
			kind = "DAYLIGHT"
		}
		lines = append(lines,
			"BEGIN:"+kind,
			"DTSTART:"+ob.localStart.Format("20060102T150405"),
			"TZOFFSETFROM:"+formatOffset(ob.fromSeconds),
			"TZOFFSETTO:"+formatOffset(ob.seconds),
			"TZNAME:"+ob.name,
			"END:"+kind,
		)
	}
	return append(lines, "END:VTIMEZONE")
}

type observance struct {
	localStart  time.Time // hora local no offset de origem
	seconds     int       // offset que passa a valer
	fromSeconds int       // offset anterior
	name        string
	dst         bool
}

// zoneObservances varre a janela atrás de mudanças de offset. Fuso estável
// rende uma única observância com o offset fixo.
func zoneObservances(loc *time.Location, from, to time.Time) []observance {
	const step = 15 * 24 * time.Hour
	_, initial := from.In(loc).Zone()
	observances := []observance{{
		localStart:  from.In(loc),
		seconds:     initial,
		fromSeconds: initial,
		name:        zoneName(from.In(loc)),
		dst:         false,
	}}
	previous := initial
	for cursor := from.Add(step); !cursor.After(to); cursor = cursor.Add(step) {
		_, offset := cursor.In(loc).Zone()
		if offset == previous {
			continue
		}
		transition := findTransition(loc, cursor.Add(-step), cursor, previous)
		local := transition.Add(time.Duration(previous) * time.Second)
		observances = append(observances, observance{
			localStart:  local,
			seconds:     offset,
			fromSeconds: previous,
			name:        zoneName(transition.In(loc)),
			dst:         offset > previous,
		})
		previous = offset
	}
	// Offset de verão pode ser negativo (hemisfério sul): o que define DAYLIGHT
	// é ter havido transição, não o sinal da diferença.
	if len(observances) > 1 {
		standard := observances[0].seconds
		for _, ob := range observances[1:] {
			if ob.seconds < standard {
				standard = ob.seconds
			}
		}
		for i := range observances {
			observances[i].dst = observances[i].seconds > standard
		}
	}
	sort.SliceStable(observances, func(i, j int) bool {
		return observances[i].localStart.Before(observances[j].localStart)
	})
	return observances
}

// findTransition estreita a mudança de offset até a hora exata.
func findTransition(loc *time.Location, before, after time.Time, previous int) time.Time {
	for after.Sub(before) > time.Hour {
		middle := before.Add(after.Sub(before) / 2)
		if _, offset := middle.In(loc).Zone(); offset == previous {
			before = middle
		} else {
			after = middle
		}
	}
	return after.Truncate(time.Hour)
}

func zoneName(t time.Time) string {
	name, _ := t.Zone()
	return name
}

// formatOffset escreve ±HHMM, como o RFC 5545 exige.
func formatOffset(seconds int) string {
	sign := "+"
	if seconds < 0 {
		sign = "-"
		seconds = -seconds
	}
	return fmt.Sprintf("%s%02d%02d", sign, seconds/3600, (seconds%3600)/60)
}

// FileName monta um nome de arquivo seguro a partir do título e da data:
// sem acento, sem barra e sem espaço, curto o bastante para qualquer sistema.
func FileName(title, date string) string {
	folded := strings.Map(foldAccent, strings.ToLower(strings.TrimSpace(title)))
	var safe strings.Builder
	for _, r := range folded {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			safe.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			safe.WriteRune('-')
		}
	}
	name := strings.Trim(safe.String(), "-")
	if len(name) > 40 {
		name = strings.Trim(name[:40], "-")
	}
	if name == "" {
		name = "compromisso"
	}
	return fmt.Sprintf("%s-%s.ics", name, date)
}

// foldAccent reduz os acentos do pt-BR ao equivalente ASCII: um nome de arquivo
// com acento vira mojibake em alguns clientes de e-mail e sistemas de arquivos.
func foldAccent(r rune) rune {
	switch r {
	case 'á', 'à', 'â', 'ã', 'ä', 'å':
		return 'a'
	case 'é', 'è', 'ê', 'ë':
		return 'e'
	case 'í', 'ì', 'î', 'ï':
		return 'i'
	case 'ó', 'ò', 'ô', 'õ', 'ö':
		return 'o'
	case 'ú', 'ù', 'û', 'ü':
		return 'u'
	case 'ç':
		return 'c'
	case 'ñ':
		return 'n'
	default:
		return r
	}
}
