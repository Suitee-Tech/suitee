package clients

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const upstreamErrorBodyLimit = 16 * 1024

// drainAndClose drains the response body (so the underlying TCP connection
// can be returned to the keep-alive pool) and then closes it. Bare
// `defer resp.Body.Close()` leaks idle sockets on error paths — under load
// that exhausts file descriptors and stalls every subsequent HTTP call out
// of the same process.
func drainAndClose(body io.ReadCloser) {
	if body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, body)
	_ = body.Close()
}

func formatUpstreamStatusError(resp *http.Response) error {
	status := resp.StatusCode
	defer drainAndClose(resp.Body)

	raw, err := io.ReadAll(io.LimitReader(resp.Body, upstreamErrorBodyLimit))
	if err != nil {
		return fmt.Errorf("upstream returned status: %d", status)
	}

	message := upstreamErrorMessage(raw)
	if message == "" {
		return fmt.Errorf("upstream returned status: %d", status)
	}
	return fmt.Errorf("upstream returned status %d: %s", status, message)
}

func upstreamErrorMessage(raw []byte) string {
	message := strings.TrimSpace(string(raw))
	if message == "" {
		return ""
	}

	var apiResponse struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(raw, &apiResponse); err == nil {
		if strings.TrimSpace(apiResponse.Message) != "" {
			return strings.TrimSpace(apiResponse.Message)
		}
		if strings.TrimSpace(apiResponse.Error) != "" {
			return strings.TrimSpace(apiResponse.Error)
		}
	}

	return message
}
