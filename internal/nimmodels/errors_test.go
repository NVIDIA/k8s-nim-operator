package nimmodels

import (
	"fmt"
	"net/http"
	"testing"
)

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		description string
		err         error
		expected    bool
	}{
		{
			description: "no error",
			err:         nil,
			expected:    false,
		},
		{
			description: "404 HTTP StatusCode",
			err:         &APIError{StatusCode: http.StatusNotFound, Status: "404 Not Found"},
			expected:    true,
		},
		{
			description: "500 HTTP StatusCode",
			err:         &APIError{StatusCode: http.StatusInternalServerError, Status: "500 Internal Server Error"},
			expected:    false,
		},
		{
			description: "generic error",
			err:         fmt.Errorf("some other error"),
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			result := IsNotFound(tt.err)
			if result != tt.expected {
				t.Errorf("IsNotFound() = %v, want %v", result, tt.expected)
			}
		})
	}
}
