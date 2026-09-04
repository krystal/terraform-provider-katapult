package vcrtest

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/dnaeon/go-vcr/cassette"
)

const redactedValue = "[REDACTED]"

var sensitiveResponseFields = map[string]struct{}{
	"backend_certificate_key": {},
	"certificate_api_url":     {},
	"initial_root_password":   {},
	"private_key":             {},
}

// RedactSensitiveResponseFields removes secret string values from JSON API
// responses before a VCR cassette is saved.
func RedactSensitiveResponseFields(i *cassette.Interaction) error {
	bodyBytes := []byte(i.Response.Body)
	if !json.Valid(bodyBytes) {
		return nil
	}

	var body any
	decoder := json.NewDecoder(bytes.NewReader(bodyBytes))
	decoder.UseNumber()
	if err := decoder.Decode(&body); err != nil {
		return fmt.Errorf("decode VCR response: %w", err)
	}

	if !redactSensitiveFields(body, sensitiveResponseFields) {
		return nil
	}

	redactedBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal redacted VCR response: %w", err)
	}

	i.Response.Body = string(redactedBody)

	return nil
}

// redactSensitiveFields replaces string values stored under the given keys
// anywhere in a decoded JSON document, along with protected installation
// attribute values. It reports whether anything changed.
func redactSensitiveFields(value any, fields map[string]struct{}) bool {
	switch value := value.(type) {
	case map[string]any:
		changed := false
		if protect, ok := value["protect"].(bool); ok && protect {
			if protected, exists := value["value"]; exists && protected != nil {
				text, isString := protected.(string)
				if (!isString || text != "") && text != redactedValue {
					value["value"] = redactedValue
					changed = true
				}
			}
		}
		for key, child := range value {
			if _, sensitive := fields[key]; sensitive {
				text, ok := child.(string)
				if ok && text != "" && text != redactedValue {
					value[key] = redactedValue
					changed = true
				}

				continue
			}

			if redactSensitiveFields(child, fields) {
				changed = true
			}
		}

		return changed
	case []any:
		changed := false
		for _, child := range value {
			if redactSensitiveFields(child, fields) {
				changed = true
			}
		}

		return changed
	default:
		return false
	}
}
