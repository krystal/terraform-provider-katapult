package vcrtest

import (
	"encoding/json"
	"fmt"

	"github.com/dnaeon/go-vcr/cassette"
)

const redactedValue = "[REDACTED]"

var sensitiveResponseFields = map[string]struct{}{
	"backend_certificate_key": {},
	"initial_root_password":   {},
}

// RedactSensitiveResponseFields removes secret string values from JSON API
// responses before a VCR cassette is saved.
func RedactSensitiveResponseFields(i *cassette.Interaction) error {
	bodyBytes := []byte(i.Response.Body)
	if !json.Valid(bodyBytes) {
		return nil
	}

	var body any
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		return fmt.Errorf("unmarshal VCR response: %w", err)
	}

	if !redactSensitiveFields(body) {
		return nil
	}

	redactedBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal redacted VCR response: %w", err)
	}

	i.Response.Body = string(redactedBody)

	return nil
}

func redactSensitiveFields(value any) bool {
	switch value := value.(type) {
	case map[string]any:
		changed := false
		for key, child := range value {
			if _, sensitive := sensitiveResponseFields[key]; sensitive {
				text, ok := child.(string)
				if ok && text != "" && text != redactedValue {
					value[key] = redactedValue
					changed = true
				}

				continue
			}

			if redactSensitiveFields(child) {
				changed = true
			}
		}

		return changed
	case []any:
		changed := false
		for _, child := range value {
			if redactSensitiveFields(child) {
				changed = true
			}
		}

		return changed
	default:
		return false
	}
}
