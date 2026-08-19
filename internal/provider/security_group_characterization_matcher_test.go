package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/dnaeon/go-vcr/cassette"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	securityGroupAssociationsJSONField = "associations"
	securityGroupTargetsJSONField      = "targets"
)

func newSecurityGroupCharacterizationTestTools(t *testing.T) *testTools {
	t.Helper()

	tt := newTestTools(t)
	if tt.Recorder != nil {
		tt.Recorder.SetMatcher(securityGroupJSONMatcher)
	}

	return tt
}

func securityGroupJSONMatcher(
	request *http.Request,
	recorded cassette.Request,
) bool {
	body, err := readAndRestoreRequestBody(request)
	if err != nil {
		return false
	}

	if !cassette.DefaultMatcher(request, recorded) {
		return false
	}

	return securityGroupJSONBodiesEqual(body, []byte(recorded.Body))
}

func readAndRestoreRequestBody(request *http.Request) ([]byte, error) {
	if request.Body == nil {
		return nil, nil
	}

	body, err := io.ReadAll(request.Body)
	request.Body = io.NopCloser(bytes.NewReader(body))

	return body, err
}

func securityGroupJSONBodiesEqual(actual, recorded []byte) bool {
	actual = bytes.TrimSpace(actual)
	recorded = bytes.TrimSpace(recorded)

	if len(actual) == 0 || len(recorded) == 0 {
		return len(actual) == len(recorded)
	}

	var actualJSON any
	if err := json.Unmarshal(actual, &actualJSON); err != nil {
		return false
	}

	var recordedJSON any
	if err := json.Unmarshal(recorded, &recordedJSON); err != nil {
		return false
	}

	return reflect.DeepEqual(
		normalizeSecurityGroupJSON(actualJSON, ""),
		normalizeSecurityGroupJSON(recordedJSON, ""),
	)
}

func normalizeSecurityGroupJSON(value any, field string) any {
	switch value := value.(type) {
	case map[string]any:
		normalized := make(map[string]any, len(value))
		for key, child := range value {
			normalized[key] = normalizeSecurityGroupJSON(child, key)
		}

		return normalized
	case []any:
		normalized := make([]any, len(value))
		allStrings := true
		for i, child := range value {
			normalized[i] = normalizeSecurityGroupJSON(child, field)
			if _, ok := normalized[i].(string); !ok {
				allStrings = false
			}
		}

		if allStrings && securityGroupJSONSetField(field) {
			sort.Slice(normalized, func(i, j int) bool {
				return normalized[i].(string) < normalized[j].(string)
			})
		}

		return normalized
	default:
		return value
	}
}

func securityGroupJSONSetField(field string) bool {
	switch field {
	case securityGroupAssociationsJSONField, securityGroupTargetsJSONField:
		return true
	default:
		return false
	}
}

func TestSecurityGroupJSONMatcher(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		method       string
		url          string
		body         string
		recorded     cassette.Request
		want         bool
		wantReadable bool
	}{
		"semantic JSON and reordered sets": {
			method: http.MethodPost,
			url:    "https://api.example.test/security_groups/rules",
			body: `{
				"properties": {
					"protocol": "TCP",
					"targets": ["all:ipv6", "all:ipv4"],
					"associations": ["vm_2", "vm_1"]
				}
			}`,
			recorded: cassette.Request{
				Method: http.MethodPost,
				URL:    "https://api.example.test/security_groups/rules",
				Body: `{
					"properties": {
						"associations": ["vm_1", "vm_2"],
						"targets": ["all:ipv4", "all:ipv6"],
						"protocol": "TCP"
					}
				}`,
			},
			want:         true,
			wantReadable: true,
		},
		"material payload difference": {
			method: http.MethodPatch,
			url:    "https://api.example.test/security_groups/rules/_?id=sgr_1",
			body:   `{"properties":{"protocol":"TCP","targets":["all:ipv4"]}}`,
			recorded: cassette.Request{
				Method: http.MethodPatch,
				URL:    "https://api.example.test/security_groups/rules/_?id=sgr_1",
				Body:   `{"properties":{"protocol":"UDP","targets":["all:ipv4"]}}`,
			},
			want:         false,
			wantReadable: true,
		},
		"method difference": {
			method: http.MethodPost,
			url:    "https://api.example.test/security_groups",
			body:   `{"properties":{"name":"web"}}`,
			recorded: cassette.Request{
				Method: http.MethodPatch,
				URL:    "https://api.example.test/security_groups",
				Body:   `{"properties":{"name":"web"}}`,
			},
			want:         false,
			wantReadable: true,
		},
		"URL difference": {
			method: http.MethodDelete,
			url:    "https://api.example.test/security_groups/_?id=sg_1",
			recorded: cassette.Request{
				Method: http.MethodDelete,
				URL:    "https://api.example.test/security_groups/_?id=sg_2",
			},
			want: false,
		},
		"empty bodies": {
			method: http.MethodGet,
			url:    "https://api.example.test/security_groups/_?id=sg_1",
			recorded: cassette.Request{
				Method: http.MethodGet,
				URL:    "https://api.example.test/security_groups/_?id=sg_1",
			},
			want: true,
		},
		"empty and non-empty bodies": {
			method: http.MethodPost,
			url:    "https://api.example.test/security_groups",
			recorded: cassette.Request{
				Method: http.MethodPost,
				URL:    "https://api.example.test/security_groups",
				Body:   `{"properties":{"name":"web"}}`,
			},
			want: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			request, err := http.NewRequestWithContext(
				context.Background(),
				test.method,
				test.url,
				strings.NewReader(test.body),
			)
			require.NoError(t, err)

			assert.Equal(
				t, test.want,
				securityGroupJSONMatcher(request, test.recorded),
			)

			if test.wantReadable {
				body, err := io.ReadAll(request.Body)
				require.NoError(t, err)
				assert.JSONEq(t, test.body, string(body))
			}
		})
	}
}
