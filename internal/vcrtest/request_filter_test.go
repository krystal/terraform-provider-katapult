package vcrtest

import (
	"testing"

	"github.com/dnaeon/go-vcr/cassette"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedactSensitiveRequestFields(t *testing.T) {
	t.Parallel()

	interaction := &cassette.Interaction{
		Request: cassette.Request{
			Body: `{
				"organization": {"sub_domain": "example"},
				"properties": {
					"issuer": "custom",
					"certificate": "-----BEGIN CERTIFICATE-----\npublic\n",
					"private_key": "-----BEGIN PRIVATE KEY-----\nsecret\n",
					"chain": null
				}
			}`,
		},
		Response: cassette.Response{Body: `{"private_key": "kept"}`},
	}

	require.NoError(t, RedactSensitiveRequestFields(interaction))
	assert.JSONEq(t, `{
		"organization": {"sub_domain": "example"},
		"properties": {
			"issuer": "custom",
			"certificate": "-----BEGIN CERTIFICATE-----\npublic\n",
			"private_key": "[REDACTED]",
			"chain": null
		}
	}`, interaction.Request.Body)
	assert.Equal(t, `{"private_key": "kept"}`, interaction.Response.Body,
		"request filter must not touch the response body")
}

func TestRedactSensitiveRequestFieldsLeavesOtherBodiesUnchanged(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"non-sensitive JSON": `{"properties":{"name":"example.com"}}`,
		"non-JSON request":   `name=example`,
		"empty request":      ``,
		"already redacted":   `{"private_key":"[REDACTED]"}`,
		"null private key":   `{"private_key":null}`,
	}

	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			interaction := &cassette.Interaction{
				Request: cassette.Request{Body: body},
			}

			require.NoError(t, RedactSensitiveRequestFields(interaction))
			assert.Equal(t, body, interaction.Request.Body)
		})
	}
}
