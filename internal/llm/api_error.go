package llm

import (
	"errors"
	"fmt"
	"strings"
)

type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	if e == nil {
		return "API error"
	}

	body := strings.TrimSpace(e.Body)
	if body == "" {
		return fmt.Sprintf("API error (status %d)", e.StatusCode)
	}

	return fmt.Sprintf("API error (status %d): %s", e.StatusCode, body)
}

func ErrorStatusCode(err error) (int, bool) {
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr == nil {
		return 0, false
	}

	if apiErr.StatusCode <= 0 {
		return 0, false
	}

	return apiErr.StatusCode, true
}
