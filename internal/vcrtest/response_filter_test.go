package vcrtest

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"

	"github.com/dnaeon/go-vcr/cassette"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	privateKeyPattern = regexp.MustCompile(
		`-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----`,
	)
	sensitiveStringFieldPattern = regexp.MustCompile(
		`"(backend_certificate_key|initial_root_password)"\s*:\s*"([^"]*)"`,
	)
)

func TestRedactSensitiveResponseFields(t *testing.T) {
	t.Parallel()

	interaction := &cassette.Interaction{
		Response: cassette.Response{
			Body: `{
				"load_balancer": {
					"backend_certificate_key": "private key",
					"name": "example"
				},
				"virtual_machines": [
					{"initial_root_password": "password"},
					{"initial_root_password": null},
					{"initial_root_password": ""}
				]
			}`,
		},
	}

	require.NoError(t, RedactSensitiveResponseFields(interaction))
	assert.JSONEq(t, `{
		"load_balancer": {
			"backend_certificate_key": "[REDACTED]",
			"name": "example"
		},
		"virtual_machines": [
			{"initial_root_password": "[REDACTED]"},
			{"initial_root_password": null},
			{"initial_root_password": ""}
		]
	}`, interaction.Response.Body)
}

func TestRedactSensitiveResponseFieldsLeavesOtherBodiesUnchanged(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"non-sensitive JSON": `{ "name": "example" }`,
		"non-JSON response":  `service unavailable`,
		"empty response":     ``,
		"already redacted":   `{"backend_certificate_key":"[REDACTED]"}`,
	}

	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			interaction := &cassette.Interaction{
				Response: cassette.Response{Body: body},
			}

			require.NoError(t, RedactSensitiveResponseFields(interaction))
			assert.Equal(t, body, interaction.Response.Body)
		})
	}
}

func TestRedactSensitiveResponseFieldsPreservesNumberLiterals(t *testing.T) {
	t.Parallel()

	interaction := &cassette.Interaction{
		Response: cassette.Response{
			Body: `{"initial_root_password":"password","id":9007199254740993}`,
		},
	}

	require.NoError(t, RedactSensitiveResponseFields(interaction))
	assert.Equal(
		t,
		`{"id":9007199254740993,"initial_root_password":"[REDACTED]"}`,
		interaction.Response.Body,
	)
}

func TestCassettesDoNotContainSensitiveResponseValues(t *testing.T) {
	t.Parallel()

	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	repositoryRoot := filepath.Clean(
		filepath.Join(filepath.Dir(filename), "..", ".."),
	)

	for _, directory := range []string{
		filepath.Join(repositoryRoot, "internal", "provider", "testdata"),
		filepath.Join(repositoryRoot, "internal", "v6provider", "testdata"),
	} {
		require.NoError(t, filepath.WalkDir(
			directory,
			func(path string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}

				if entry.IsDir() || filepath.Ext(path) != ".yaml" {
					return nil
				}

				contents, err := os.ReadFile(path)
				require.NoError(t, err)

				relativePath, err := filepath.Rel(repositoryRoot, path)
				require.NoError(t, err)
				assert.NotRegexp(
					t,
					privateKeyPattern,
					string(contents),
					relativePath,
				)

				matches := sensitiveStringFieldPattern.FindAllSubmatch(
					contents,
					-1,
				)
				for _, match := range matches {
					assert.Contains(
						t,
						[]string{"", redactedValue},
						string(match[2]),
						"%s contains an unredacted %s value",
						relativePath,
						match[1],
					)
				}

				return nil
			},
		))
	}
}
