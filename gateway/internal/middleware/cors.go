package middleware

import (
	"errors"
	"net/url"
	"strings"

	"github.com/oginoino/toca/pkg/config"
)

func AllowedCORSOrigins(appEnv string, appMode string, allowedOrigins string, frontendCORSURL string) ([]string, error) {
	// P2/M16: critério único de release (pkg/config), sem cópias divergentes.
	isRelease := config.IsReleaseEnv(appEnv, appMode)

	parsedOrigins := make([]string, 0)
	for _, o := range strings.Split(allowedOrigins, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			parsedOrigins = append(parsedOrigins, o)
		}
	}

	if isRelease {
		if len(parsedOrigins) == 0 {
			return nil, errors.New("CORS origins must be configured via CORS_ALLOWED_ORIGINS/ALLOWED_ORIGINS in release mode")
		}
		for _, o := range parsedOrigins {
			if o == "*" {
				return nil, errors.New("CORS wildcard origin is not allowed in release mode")
			}
		}
		return parsedOrigins, nil
	}

	if len(parsedOrigins) > 0 {
		return parsedOrigins, nil
	}

	return buildDevCorsOrigins(frontendCORSURL), nil
}

func buildDevCorsOrigins(frontendURL string) []string {
	out := make([]string, 0)
	seen := map[string]struct{}{}
	add := func(origin string) {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			return
		}
		if _, ok := seen[origin]; ok {
			return
		}
		seen[origin] = struct{}{}
		out = append(out, origin)
	}

	add(frontendURL)
	add("http://localhost:5173")
	add("http://localhost:5174")
	add("http://127.0.0.1:5173")
	add("http://127.0.0.1:5174")

	if frontendURL != "" {
		if u, err := url.Parse(frontendURL); err == nil {
			scheme := u.Scheme
			if scheme == "" {
				scheme = "http"
			}
			host := u.Hostname()
			if host == "localhost" || host == "127.0.0.1" {
				add((&url.URL{Scheme: scheme, Host: host + ":5173"}).String())
				add((&url.URL{Scheme: scheme, Host: host + ":5174"}).String())
			}
		}
	}

	return out
}
