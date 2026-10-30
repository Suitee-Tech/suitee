package icalendar

import (
	"strings"
	"testing"
	"time"
)

func saoPaulo(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatalf("fuso indisponível: %v", err)
	}
	return loc
}

func TestRenderCalendarFileFollowsRFC5545(t *testing.T) {
	loc := saoPaulo(t)
	start := time.Date(2026, 10, 10, 14, 0, 0, 0, loc)
	file, err := Render(Copy{
		UID: "appt-1@suitee", Title: "Visita; imóvel, vista \\ mar",
		Location: "Apto Jardins, torre A", Start: start, End: start.Add(time.Hour),
		TimeZone: "America/Sao_Paulo", Stamp: start.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	body := string(file)
	if !strings.HasPrefix(body, "BEGIN:VCALENDAR\r\n") {
		t.Error("arquivo precisa começar com BEGIN:VCALENDAR em CRLF")
	}
	if !strings.HasSuffix(body, "END:VCALENDAR\r\n") {
		t.Error("arquivo precisa terminar com END:VCALENDAR em CRLF")
	}
	if strings.Contains(strings.ReplaceAll(body, "\r\n", ""), "\n") {
		t.Error("toda quebra de linha precisa ser CRLF")
	}
	for _, want := range []string{
		"VERSION:2.0",
		"METHOD:PUBLISH",
		"UID:appt-1@suitee",
		"DTSTAMP:20261010T160000Z",
		"DTSTART;TZID=America/Sao_Paulo:20261010T140000",
		"DTEND;TZID=America/Sao_Paulo:20261010T150000",
		"SUMMARY:Visita\\; imóvel\\, vista \\\\ mar",
		"LOCATION:Apto Jardins\\, torre A",
		"STATUS:CONFIRMED",
		"BEGIN:VTIMEZONE",
		"TZID:America/Sao_Paulo",
		"TZOFFSETTO:-0300",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("faltou %q no arquivo:\n%s", want, body)
		}
	}
	// Sem convidados nem RSVP: é uma cópia, não um convite.
	for _, forbidden := range []string{"ATTENDEE", "METHOD:REQUEST", "ORGANIZER", "DESCRIPTION"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("arquivo não pode conter %s", forbidden)
		}
	}
}

func TestRenderCalendarFileFoldsLongLines(t *testing.T) {
	loc := saoPaulo(t)
	start := time.Date(2026, 10, 10, 14, 0, 0, 0, loc)
	long := strings.Repeat("descrição muito longa do compromisso ", 6)
	file, err := Render(Copy{
		UID: "appt-1@suitee", Title: long, Start: start, End: start.Add(time.Hour),
		TimeZone: "America/Sao_Paulo",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, line := range strings.Split(string(file), "\r\n") {
		if len(line) > maxLineOctets {
			t.Fatalf("linha com %d octetos passou do limite: %q", len(line), line)
		}
	}
	// A dobra precisa continuar sendo o mesmo texto para quem lê.
	unfolded := strings.ReplaceAll(string(file), "\r\n ", "")
	if !strings.Contains(unfolded, "SUMMARY:"+escapeText(long)) {
		t.Error("dobra quebrou o conteúdo do resumo")
	}
}

func TestRenderCalendarFileCarriesDaylightSavingRules(t *testing.T) {
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("tzdata indisponível")
	}
	start := time.Date(2026, 7, 10, 14, 0, 0, 0, newYork)
	file, err := Render(Copy{
		UID: "appt-1@suitee", Title: "Visita", Start: start, End: start.Add(time.Hour),
		TimeZone: "America/New_York",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	body := string(file)
	for _, want := range []string{"BEGIN:DAYLIGHT", "TZOFFSETTO:-0400", "TZOFFSETFROM:-0500", "BEGIN:STANDARD", "TZOFFSETTO:-0500"} {
		if !strings.Contains(body, want) {
			t.Errorf("faltou %q no VTIMEZONE:\n%s", want, body)
		}
	}
}

func TestRenderCalendarFileRejectsIncompleteCopy(t *testing.T) {
	start := time.Date(2026, 10, 10, 14, 0, 0, 0, time.UTC)
	for _, copy := range []Copy{
		{Title: "sem uid", Start: start, End: start.Add(time.Hour), TimeZone: "UTC"},
		{UID: "u", Start: start, End: start.Add(time.Hour)},
		{UID: "u", Start: start, End: start, TimeZone: "UTC"},
		{UID: "u", Start: start, End: start.Add(time.Hour), TimeZone: "Marte/Olympus"},
	} {
		if _, err := Render(copy); err == nil {
			t.Errorf("cópia inválida aceita sem erro: %+v", copy)
		}
	}
}
