package provider

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_parseGenericAPIError(t *testing.T) {
	tests := []struct {
		name string
		body []byte
		want *GenericAPIError
	}{
		{
			name: "valid error response",
			body: []byte(`{
				"error": {
					"code": "not_found",
					"description": "Resource not found"
				}
			}`),
			want: &GenericAPIError{
				Code:        "not_found",
				Description: "Resource not found",
			},
		},
		{
			name: "error with detail array",
			body: []byte(`{
				"error": {
					"code": "not_found",
					"description": "Resource not found",
					"detail": ["this is", "an array"]
				}
			}`),
			want: &GenericAPIError{
				Code:        "not_found",
				Description: "Resource not found",
				Detail:      "this is, an array",
			},
		},
		{
			name: "error with detail object",
			body: []byte(`{
				"error": {
					"code": "not_found",
					"description": "Resource not found",
					"detail": {"scope": "global", "data": "none"}
				}
			}`),
			want: &GenericAPIError{
				Code:        "not_found",
				Description: "Resource not found",
				Detail:      "data=none, scope=global",
			},
		},
		{
			name: "missing error code",
			body: []byte(`{
				"error": {
					"description": "Something went wrong"
				}
			}`),
			want: nil,
		},
		{
			name: "invalid json",
			body: []byte(`not json`),
			want: nil,
		},
		{
			name: "empty response",
			body: []byte(``),
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseGenericAPIError(tt.body)

			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_genericAPIError(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		body    []byte
		wantErr error
	}{
		{
			name: "valid API error",
			err:  errors.New("original error"),
			body: []byte(`{
				"error": {
					"code": "not_found",
					"description": "Resource not found"
				}
			}`),
			wantErr: &GenericAPIError{
				Code:        "not_found",
				Description: "Resource not found",
			},
		},
		{
			name:    "no API error returns original error",
			err:     errors.New("original error"),
			body:    []byte(`{}`),
			wantErr: errors.New("original error"),
		},
		{
			name:    "empty body returns original error",
			err:     errors.New("original error"),
			body:    []byte(``),
			wantErr: errors.New("original error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := genericAPIError(tt.err, tt.body)

			if tt.wantErr == nil {
				assert.NoError(t, got)
			} else {
				var apiErr *GenericAPIError
				if errors.As(tt.wantErr, &apiErr) {
					assert.Equal(t, apiErr, got)
				} else {
					assert.Equal(t, tt.wantErr.Error(), got.Error())
				}
			}
		})
	}
}
