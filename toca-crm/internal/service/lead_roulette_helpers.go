package service

import (
	"strings"
)

func hasIntersection(a []string, b []string) bool {
	return countIntersect(a, b) > 0
}

func countIntersect(a []string, b []string) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}

	set := map[string]bool{}
	for _, v := range a {
		v = strings.TrimSpace(strings.ToUpper(v))
		if v != "" {
			set[v] = true
		}
	}
	if len(set) == 0 {
		return 0
	}

	cnt := 0
	seen := map[string]bool{}
	for _, v := range b {
		v = strings.TrimSpace(strings.ToUpper(v))
		if v == "" {
			continue
		}
		if seen[v] {
			continue
		}
		seen[v] = true
		if set[v] {
			cnt++
		}
	}
	return cnt
}

func regionMatches(regions []string, preferredLocation string) bool {
	if len(regions) == 0 {
		return false
	}
	preferredLocation = strings.TrimSpace(preferredLocation)
	if preferredLocation == "" {
		return false
	}

	locationTokens := strings.FieldsFunc(preferredLocation, func(r rune) bool {
		switch r {
		case ',', '-', '/':
			return true
		default:
			return false
		}
	})
	locationSet := map[string]bool{}
	for _, t := range locationTokens {
		t = strings.TrimSpace(strings.ToUpper(t))
		if t != "" {
			locationSet[t] = true
		}
	}
	if len(locationSet) == 0 {
		return false
	}

	for _, r := range regions {
		r = strings.TrimSpace(strings.ToUpper(r))
		if r == "" {
			continue
		}
		if locationSet[r] {
			return true
		}
	}
	return false
}

func extractQualificationValues(q map[string]interface{}, fieldIDs []string) []string {
	if q == nil || len(fieldIDs) == 0 {
		return nil
	}
	out := make([]string, 0, 8)
	seen := map[string]bool{}
	for _, fid := range fieldIDs {
		fid = strings.TrimSpace(fid)
		if fid == "" {
			continue
		}
		v, ok := q[fid]
		if !ok || v == nil {
			continue
		}
		var vals []string
		switch t := v.(type) {
		case string:
			vals = []string{t}
		case []string:
			vals = t
		case []interface{}:
			for _, it := range t {
				if s, ok := it.(string); ok {
					vals = append(vals, s)
				}
			}
		default:
			continue
		}
		for _, s := range vals {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			k := strings.ToUpper(s)
			if seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, s)
		}
	}
	return out
}

func extractQualificationLocation(q map[string]interface{}, fieldID string) string {
	fieldID = strings.TrimSpace(fieldID)
	if fieldID == "" {
		return ""
	}
	vals := extractQualificationValues(q, []string{fieldID})
	if len(vals) == 0 {
		return ""
	}
	return strings.Join(vals, ", ")
}
