package handlers

import "strings"

func isNotImplemented(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not implemented") && strings.Contains(msg, "postgres") && strings.Contains(msg, "pilot")
}
