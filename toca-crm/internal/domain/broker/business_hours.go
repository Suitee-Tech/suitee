package broker

import (
	"fmt"
	"time"
)

type BusinessHours struct {
	Timezone string                `json:"timezone,omitempty"`
	Windows  map[string]TimeWindow `json:"windows,omitempty"`
}

type TimeWindow struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

func (bh *BusinessHours) IsOpenAt(t time.Time) bool {
	if bh == nil {
		return true
	}

	loc := time.UTC
	if bh.Timezone != "" {
		if l, err := time.LoadLocation(bh.Timezone); err == nil && l != nil {
			loc = l
		}
	}

	local := t.In(loc)
	dayKey := fmt.Sprintf("%d", int(local.Weekday()))
	w, ok := bh.Windows[dayKey]
	if !ok || w.Start == "" || w.End == "" {
		return false
	}

	startT, err := time.ParseInLocation("15:04", w.Start, loc)
	if err != nil {
		return false
	}
	endT, err := time.ParseInLocation("15:04", w.End, loc)
	if err != nil {
		return false
	}

	y, m, d := local.Date()
	start := time.Date(y, m, d, startT.Hour(), startT.Minute(), 0, 0, loc)
	end := time.Date(y, m, d, endT.Hour(), endT.Minute(), 0, 0, loc)
	if !end.After(start) {
		return false
	}

	return !local.Before(start) && local.Before(end)
}
