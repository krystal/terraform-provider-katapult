package vcrtest

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/dnaeon/go-vcr/cassette"
)

var sensitiveRequestFields = map[string]struct{}{
	"private_key": {},
}

// RedactSensitiveRequestFields removes secret string values from JSON API
// request bodies before a VCR cassette is saved, so configured private keys
// never land in a cassette. The default go-vcr matcher compares only method
// and URL, so rewriting request bodies does not affect replay.
func RedactSensitiveRequestFields(i *cassette.Interaction) error {
	bodyBytes := []byte(i.Request.Body)
	if !json.Valid(bodyBytes) {
		return nil
	}

	var body any
	decoder := json.NewDecoder(bytes.NewReader(bodyBytes))
	decoder.UseNumber()
	if err := decoder.Decode(&body); err != nil {
		return fmt.Errorf("decode VCR request: %w", err)
	}

	if !redactSensitiveFields(body, sensitiveRequestFields) {
		return nil
	}

	redactedBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal redacted VCR request: %w", err)
	}

	i.Request.Body = string(redactedBody)

	return nil
}
