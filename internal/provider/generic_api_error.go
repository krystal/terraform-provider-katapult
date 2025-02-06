package provider

import (
	"sort"
	"strings"

	"github.com/tidwall/gjson"
)

type GenericAPIError struct {
	Code        string
	Description string
	Detail      string
}

func (e *GenericAPIError) Error() string {
	r := e.Code

	if e.Description != "" {
		r += ": " + e.Description
	}

	if e.Detail != "" {
		r += ": " + e.Detail
	}

	return r
}

func genericAPIError(err error, body []byte) error {
	apiErr := parseGenericAPIError(body)
	if apiErr != nil {
		return apiErr
	}

	return err
}

func parseGenericAPIError(body []byte) *GenericAPIError {
	gj := gjson.ParseBytes(body)

	code := gj.Get("error.code").String()
	if code == "" {
		return nil
	}

	err := &GenericAPIError{
		Code:        code,
		Description: gj.Get("error.description").String(),
	}

	if detail := gj.Get("error.detail"); detail.Exists() {
		var values []string
		switch {
		case detail.IsArray():
			detail.ForEach(func(_, v gjson.Result) bool {
				values = append(values, v.String())

				return true
			})
		case detail.IsObject():
			for k, v := range detail.Map() {
				values = append(values, k+"="+v.String())
			}
			sort.Strings(values)
		default:
			values = append(values, detail.String())
		}

		err.Detail = strings.Join(values, ", ")
	}

	return err
}
