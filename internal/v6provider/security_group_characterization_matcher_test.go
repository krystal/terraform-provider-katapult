package v6provider

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/dnaeon/go-vcr/cassette"
	"github.com/krystal/go-katapult/next/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type orderedSecurityGroupCassetteTransport struct {
	mu             sync.Mutex
	interactions   map[string][]*cassette.Interaction
	next           map[string]int
	consumed       map[string]map[int]bool
	ruleSnapshots  map[string]securityGroupReplaySnapshot
	deletedRules   map[string]bool
	groupSnapshots map[string]securityGroupReplaySnapshot
	deletedGroups  map[string]bool
	ruleBodies     map[string]json.RawMessage
	ruleGroups     map[string]string
	ruleOrder      []string
}

type securityGroupReplaySnapshot struct {
	status  string
	code    int
	headers http.Header
	body    string
}

func newOrderedSecurityGroupCassetteTransport(t *testing.T) http.RoundTripper {
	t.Helper()

	loaded, err := cassette.Load(testDataFilePath(t, ".cassette"))
	require.NoError(t, err)

	transport := &orderedSecurityGroupCassetteTransport{
		interactions:   make(map[string][]*cassette.Interaction),
		next:           make(map[string]int),
		consumed:       make(map[string]map[int]bool),
		ruleSnapshots:  make(map[string]securityGroupReplaySnapshot),
		deletedRules:   make(map[string]bool),
		groupSnapshots: make(map[string]securityGroupReplaySnapshot),
		deletedGroups:  make(map[string]bool),
		ruleBodies:     make(map[string]json.RawMessage),
		ruleGroups:     make(map[string]string),
	}
	for _, interaction := range loaded.Interactions {
		key := securityGroupCassetteRequestKey(
			interaction.Method,
			interaction.URL,
			[]byte(interaction.Request.Body),
		)
		transport.interactions[key] = append(transport.interactions[key], interaction)
	}

	return transport
}

//nolint:gocyclo // Ordered legacy replay deliberately handles route and state aliases together.
func (transport *orderedSecurityGroupCassetteTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	transport.mu.Lock()
	defer transport.mu.Unlock()

	body, err := readAndRestoreRequestBody(request)
	if err != nil {
		return nil, err
	}
	if ruleID := securityGroupRuleRequestID(request, body); ruleID != "" {
		if request.Method == http.MethodGet {
			if transport.deletedRules[ruleID] {
				return replayDeletedSecurityGroupRule(request), nil
			}
			if snapshot, ok := transport.ruleSnapshots[ruleID]; ok {
				return replaySecurityGroupSnapshot(request, snapshot), nil
			}
		}
		if request.Method == http.MethodDelete {
			delete(transport.ruleSnapshots, ruleID)
			transport.deletedRules[ruleID] = true
			delete(transport.ruleBodies, ruleID)
		}
	}
	if groupID := securityGroupRequestID(request, body); groupID != "" {
		if request.Method == http.MethodGet {
			if transport.deletedGroups[groupID] {
				return replayDeletedSecurityGroup(request), nil
			}
			if snapshot, ok := transport.groupSnapshots[groupID]; ok {
				return replaySecurityGroupSnapshot(request, snapshot), nil
			}
		}
		if request.Method == http.MethodDelete {
			delete(transport.groupSnapshots, groupID)
			transport.deletedGroups[groupID] = true
		}
	}
	if groupID := securityGroupRulesRequestGroupID(request, body); request.Method == http.MethodGet && groupID != "" {
		if _, known := transport.groupSnapshots[groupID]; known {
			return transport.replaySecurityGroupRules(request, groupID), nil
		}
	}
	key := securityGroupCassetteRequestKey(request.Method, request.URL.String(), body)
	candidates := transport.interactions[key]
	for index := transport.next[key]; index < len(candidates); index++ {
		if transport.consumed[key][index] {
			continue
		}
		interaction := candidates[index]
		if !securityGroupJSONMatcher(request, interaction.Request) {
			continue
		}
		if strings.Contains(interaction.Response.Body, "Ports cannot be set with ICMP") &&
			!securityGroupRequestHasNonEmptyPorts(body) {
			continue
		}
		transport.consumeInteraction(key, index)
		transport.observeMutation(request, interaction)
		return replaySecurityGroupInteraction(request, interaction), nil
	}
	for index := transport.next[key]; index < len(candidates); index++ {
		if transport.consumed[key][index] {
			continue
		}
		interaction := candidates[index]
		if request.Method != http.MethodPost ||
			!strings.HasSuffix(request.URL.Path, "/security_groups") ||
			!securityGroupCreateBodiesCompatible(body, []byte(interaction.Request.Body)) {
			continue
		}
		transport.consumeInteraction(key, index)
		transport.observeMutation(request, interaction)
		return replaySecurityGroupInteraction(request, interaction), nil
	}
	if len(candidates) > 0 && candidates[len(candidates)-1].Code == http.StatusNotFound &&
		securityGroupJSONMatcher(request, candidates[len(candidates)-1].Request) {
		return replaySecurityGroupInteraction(request, candidates[len(candidates)-1]), nil
	}
	if request.Method == http.MethodGet && len(candidates) > 0 &&
		securityGroupJSONMatcher(request, candidates[len(candidates)-1].Request) {
		return replaySecurityGroupInteraction(request, candidates[len(candidates)-1]), nil
	}

	return nil, cassette.ErrInteractionNotFound
}

func (transport *orderedSecurityGroupCassetteTransport) consumeInteraction(
	key string,
	index int,
) {
	if transport.consumed == nil {
		transport.consumed = make(map[string]map[int]bool)
	}
	if transport.consumed[key] == nil {
		transport.consumed[key] = make(map[int]bool)
	}
	transport.consumed[key][index] = true
	for transport.consumed[key][transport.next[key]] {
		delete(transport.consumed[key], transport.next[key])
		transport.next[key]++
	}
}

func securityGroupCreateBodiesCompatible(actual, recorded []byte) bool {
	type createRequest struct {
		Organization map[string]any `json:"organization"`
		Properties   struct {
			Name             string `json:"name"`
			Associations     []any  `json:"associations"`
			AllowAllInbound  *bool  `json:"allow_all_inbound"`
			AllowAllOutbound *bool  `json:"allow_all_outbound"`
		} `json:"properties"`
	}
	var actualRequest, recordedRequest createRequest
	decode := func(body []byte, target *createRequest) bool {
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.DisallowUnknownFields()
		if decoder.Decode(target) != nil {
			return false
		}
		return decoder.Decode(&struct{}{}) == io.EOF
	}
	if !decode(actual, &actualRequest) || !decode(recorded, &recordedRequest) {
		return false
	}
	return reflect.DeepEqual(actualRequest.Organization, recordedRequest.Organization) &&
		actualRequest.Properties.Name == recordedRequest.Properties.Name &&
		securityGroupOptionalBoolCompatible(
			actualRequest.Properties.AllowAllInbound,
			recordedRequest.Properties.AllowAllInbound,
		) &&
		securityGroupOptionalBoolCompatible(
			actualRequest.Properties.AllowAllOutbound,
			recordedRequest.Properties.AllowAllOutbound,
		) &&
		reflect.DeepEqual(
			normalizeSecurityGroupJSON(
				actualRequest.Properties.Associations,
				securityGroupAssociationsJSONField,
			),
			normalizeSecurityGroupJSON(
				recordedRequest.Properties.Associations,
				securityGroupAssociationsJSONField,
			),
		)
}

func securityGroupOptionalBoolCompatible(actual, recorded *bool) bool {
	return actual == nil || recorded == nil || *actual == *recorded
}

func (transport *orderedSecurityGroupCassetteTransport) observeMutation(
	request *http.Request,
	interaction *cassette.Interaction,
) {
	if request.Method != http.MethodPost && request.Method != http.MethodPatch {
		return
	}
	if interaction.Code < http.StatusOK || interaction.Code >= http.StatusMultipleChoices {
		return
	}

	var payload struct {
		SecurityGroupRule json.RawMessage `json:"security_group_rule"`
		SecurityGroup     *struct {
			ID string `json:"id"`
		} `json:"security_group"`
	}
	if json.Unmarshal([]byte(interaction.Response.Body), &payload) != nil {
		return
	}
	snapshot := securityGroupReplaySnapshot{
		status:  interaction.Status,
		code:    interaction.Code,
		headers: interaction.Response.Headers.Clone(),
		body:    interaction.Response.Body,
	}
	if len(payload.SecurityGroupRule) > 0 {
		var rule struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(payload.SecurityGroupRule, &rule) == nil && rule.ID != "" {
			if _, known := transport.ruleBodies[rule.ID]; !known {
				transport.ruleOrder = append(transport.ruleOrder, rule.ID)
			}
			transport.ruleSnapshots[rule.ID] = snapshot
			transport.ruleBodies[rule.ID] = append(json.RawMessage(nil), payload.SecurityGroupRule...)
			if groupID := securityGroupRuleMutationGroupID(request); groupID != "" {
				transport.ruleGroups[rule.ID] = groupID
			}
			delete(transport.deletedRules, rule.ID)
		}
	}
	if payload.SecurityGroup != nil && payload.SecurityGroup.ID != "" {
		snapshot.body = securityGroupMutationResponseBody(request, snapshot.body)
		transport.groupSnapshots[payload.SecurityGroup.ID] = snapshot
		delete(transport.deletedGroups, payload.SecurityGroup.ID)
	}
}

func securityGroupMutationResponseBody(request *http.Request, responseBody string) string {
	requestBody, err := readAndRestoreRequestBody(request)
	if err != nil || len(requestBody) == 0 {
		return responseBody
	}
	var requestPayload struct {
		Properties map[string]any `json:"properties"`
	}
	var responsePayload map[string]any
	if json.Unmarshal(requestBody, &requestPayload) != nil ||
		json.Unmarshal([]byte(responseBody), &responsePayload) != nil {
		return responseBody
	}
	group, ok := responsePayload["security_group"].(map[string]any)
	if !ok {
		return responseBody
	}
	for key, value := range requestPayload.Properties {
		group[key] = value
	}
	body, err := json.Marshal(responsePayload)
	if err != nil {
		return responseBody
	}
	return string(body)
}

func securityGroupRuleMutationGroupID(request *http.Request) string {
	if request.Body == nil {
		return ""
	}
	body, err := readAndRestoreRequestBody(request)
	if err != nil {
		return ""
	}
	var payload struct {
		SecurityGroup *struct {
			ID string `json:"id"`
		} `json:"security_group"`
	}
	if json.Unmarshal(body, &payload) == nil && payload.SecurityGroup != nil {
		return payload.SecurityGroup.ID
	}
	segments := strings.Split(strings.Trim(request.URL.Path, "/"), "/")
	for index := 0; index+2 < len(segments); index++ {
		if segments[index] == "security_groups" && segments[index+2] == "rules" &&
			segments[index+1] != "_" && segments[index+1] != "security_group" {
			return segments[index+1]
		}
	}
	return ""
}

func securityGroupRuleRequestID(request *http.Request, body []byte) string {
	value := *request.URL
	normalizeSecurityGroupURL(&value, body)
	return value.Query().Get("security_group_rule[id]")
}

func securityGroupRequestID(request *http.Request, body []byte) string {
	value := *request.URL
	normalizeSecurityGroupURL(&value, body)
	if !strings.HasSuffix(value.Path, "/security_groups/_") {
		return ""
	}
	return value.Query().Get("security_group[id]")
}

func securityGroupRulesRequestGroupID(request *http.Request, body []byte) string {
	value := *request.URL
	normalizeSecurityGroupURL(&value, body)
	if !strings.HasSuffix(value.Path, "/rules") {
		return ""
	}
	return value.Query().Get("security_group[id]")
}

func (transport *orderedSecurityGroupCassetteTransport) replaySecurityGroupRules(
	request *http.Request,
	groupID string,
) *http.Response {
	rules := make([]json.RawMessage, 0)
	for _, ruleID := range transport.ruleOrder {
		if transport.ruleGroups[ruleID] == groupID {
			if body, ok := transport.ruleBodies[ruleID]; ok {
				rules = append(rules, body)
			}
		}
	}
	payload := struct {
		Pagination struct {
			CurrentPage int  `json:"current_page"`
			TotalPages  int  `json:"total_pages"`
			Total       int  `json:"total"`
			PerPage     int  `json:"per_page"`
			LargeSet    bool `json:"large_set"`
		} `json:"pagination"`
		Rules []json.RawMessage `json:"security_group_rules"`
	}{Rules: rules}
	payload.Pagination.CurrentPage = 1
	payload.Pagination.TotalPages = 1
	payload.Pagination.Total = len(rules)
	payload.Pagination.PerPage = 30
	body, _ := json.Marshal(payload)
	return &http.Response{
		Status:     "200 OK",
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
		Request:    request,
	}
}

func replaySecurityGroupSnapshot(
	request *http.Request,
	snapshot securityGroupReplaySnapshot,
) *http.Response {
	return &http.Response{
		Status:     snapshot.status,
		StatusCode: snapshot.code,
		Header:     snapshot.headers.Clone(),
		Body:       io.NopCloser(strings.NewReader(snapshot.body)),
		Request:    request,
	}
}

func replayDeletedSecurityGroupRule(request *http.Request) *http.Response {
	return &http.Response{
		Status:     "404 Not Found",
		StatusCode: http.StatusNotFound,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"error":{"code":"security_group_rule_not_found","description":"Security group rule was not found"}}`,
		)),
		Request: request,
	}
}

func replayDeletedSecurityGroup(request *http.Request) *http.Response {
	return &http.Response{
		Status:     "404 Not Found",
		StatusCode: http.StatusNotFound,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"error":{"code":"security_group_not_found","description":"Security group was not found"}}`,
		)),
		Request: request,
	}
}

func securityGroupRequestHasNonEmptyPorts(body []byte) bool {
	var payload struct {
		Properties struct {
			Ports string `json:"ports"`
		} `json:"properties"`
	}
	return json.Unmarshal(body, &payload) == nil && payload.Properties.Ports != ""
}

func replaySecurityGroupInteraction(request *http.Request, interaction *cassette.Interaction) *http.Response {
	return &http.Response{
		Status:     interaction.Status,
		StatusCode: interaction.Code,
		Header:     interaction.Response.Headers.Clone(),
		Body:       io.NopCloser(strings.NewReader(interaction.Response.Body)),
		Request:    request,
	}
}

func securityGroupCassetteRequestKey(method, rawURL string, body []byte) string {
	value, err := url.Parse(rawURL)
	if err != nil {
		return method + " " + rawURL
	}
	normalizeSecurityGroupURL(value, body)

	return method + " " + value.String()
}

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

	recordedURL, err := url.Parse(recorded.URL)
	if err != nil || request.Method != recorded.Method {
		return false
	}
	actualURL := *request.URL
	normalizeSecurityGroupURL(&actualURL, body)
	normalizeSecurityGroupURL(recordedURL, []byte(recorded.Body))
	if actualURL.String() != recordedURL.String() {
		return false
	}

	return securityGroupJSONBodiesEqual(body, []byte(recorded.Body))
}

//nolint:lll // Full URLs and JSON make the route-alias example explicit.
func TestSecurityGroupJSONMatcherAcceptsGeneratedClientRouteAliases(t *testing.T) {
	t.Parallel()

	body := `{"organization":{"sub_domain":"terraform-acc-test"},"properties":{"name":"test","associations":[],"allow_all_inbound":false,"allow_all_outbound":false}}`
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://api.katapult.io/core/v1/organizations/organization/security_groups", strings.NewReader(body))
	require.NoError(t, err)
	recorded := cassette.Request{
		Method: http.MethodPost,
		URL:    "https://api.katapult.io/core/v1/organizations/_/security_groups",
		Body:   body,
	}

	assert.True(t, securityGroupJSONMatcher(request, recorded))
}

//nolint:lll // Route alias mappings are clearer when kept inline.
func normalizeSecurityGroupURL(value *url.URL, body []byte) {
	value.Path = strings.Replace(value.Path, "/organizations/organization/security_groups", "/organizations/_/security_groups", 1)
	var payload struct {
		SecurityGroup *struct {
			ID string `json:"id"`
		} `json:"security_group"`
		SecurityGroupRule *struct {
			ID string `json:"id"`
		} `json:"security_group_rule"`
		Properties json.RawMessage `json:"properties"`
	}
	_ = json.Unmarshal(body, &payload)
	if strings.Contains(value.Path, "/security_groups/rules/security_group_rule") {
		value.Path = strings.Replace(value.Path, "/security_groups/rules/security_group_rule", "/security_groups/rules/_", 1)
		if payload.SecurityGroupRule != nil && payload.SecurityGroupRule.ID != "" {
			query := value.Query()
			query.Set("security_group_rule[id]", payload.SecurityGroupRule.ID)
			value.RawQuery = query.Encode()
		}
	}
	if strings.HasSuffix(value.Path, "/security_groups/security_group/rules") {
		if payload.SecurityGroup != nil && payload.SecurityGroup.ID != "" {
			value.Path = strings.TrimSuffix(value.Path, "security_group/rules") + payload.SecurityGroup.ID + "/rules"
		} else {
			value.Path = strings.Replace(value.Path, "/security_groups/security_group/rules", "/security_groups/_/rules", 1)
		}
	}
	if strings.HasSuffix(value.Path, "/security_groups/security_group") && payload.SecurityGroup != nil && payload.SecurityGroup.ID != "" && len(payload.Properties) == 0 {
		query := value.Query()
		query.Set("security_group[id]", payload.SecurityGroup.ID)
		value.RawQuery = query.Encode()
	}
	value.Path = strings.Replace(value.Path, "/security_groups/security_group", "/security_groups/_", 1)
}

func readAndRestoreRequestBody(request *http.Request) ([]byte, error) {
	if request.Body == nil {
		return nil, nil
	}

	body, err := io.ReadAll(request.Body)
	request.Body = io.NopCloser(bytes.NewReader(body))

	return body, err
}

//nolint:lll,gocyclo // Compatibility matching normalizes several legacy wire differences.
func securityGroupJSONBodiesEqual(actual, recorded []byte) bool {
	actual = bytes.TrimSpace(actual)
	recorded = bytes.TrimSpace(recorded)

	if len(actual) == 0 || len(recorded) == 0 {
		if len(recorded) == 0 {
			var actualMap map[string]any
			if json.Unmarshal(actual, &actualMap) == nil {
				delete(actualMap, "security_group")
				delete(actualMap, "security_group_rule")
				return len(actualMap) == 0
			}
		}
		return len(actual) == len(recorded)
	}

	var actualJSON any
	if err := json.Unmarshal(actual, &actualJSON); err != nil {
		return bytes.Equal(actual, recorded)
	}

	var recordedJSON any
	if err := json.Unmarshal(recorded, &recordedJSON); err != nil {
		return bytes.Equal(actual, recorded)
	}
	if actualMap, actualOK := actualJSON.(map[string]any); actualOK {
		if recordedMap, recordedOK := recordedJSON.(map[string]any); recordedOK {
			actualProperties, actualHasProperties := actualMap["properties"].(map[string]any)
			recordedProperties, recordedHasProperties := recordedMap["properties"].(map[string]any)
			if actualHasProperties && recordedHasProperties {
				if action, ok := actualProperties[securityGroupActionJSONField].(string); ok &&
					action == string(core.Allow) {
					if _, recordedAction := recordedProperties[securityGroupActionJSONField]; !recordedAction {
						delete(actualProperties, securityGroupActionJSONField)
					}
				}
				for key := range actualProperties {
					if _, ok := recordedProperties[key]; !ok &&
						securityGroupLegacyOmittableJSONProperty(key) {
						delete(actualProperties, key)
					}
				}
			}
		}
	}

	if actualMap, ok := actualJSON.(map[string]any); ok && strings.Contains(string(actual), `"security_group"`) && strings.Contains(string(recorded), `"properties"`) {
		recordedMap, recordedIsMap := recordedJSON.(map[string]any)
		_, recordedHasGroup := recordedMap["security_group"]
		if recordedIsMap && !recordedHasGroup {
			delete(actualMap, "security_group")
		}
	}
	if actualMap, ok := actualJSON.(map[string]any); ok {
		recordedMap, recordedIsMap := recordedJSON.(map[string]any)
		_, recordedHasRule := recordedMap["security_group_rule"]
		if recordedIsMap && !recordedHasRule {
			delete(actualMap, "security_group_rule")
		}
	}

	return reflect.DeepEqual(
		normalizeSecurityGroupJSON(actualJSON, ""),
		normalizeSecurityGroupJSON(recordedJSON, ""),
	)
}

func securityGroupLegacyOmittableJSONProperty(name string) bool {
	switch name {
	case "name", securityGroupAssociationsJSONField,
		securityGroupAllowAllInboundJSONField, securityGroupAllowAllOutboundJSONField,
		securityGroupDirectionJSONField, "protocol", "ports", securityGroupTargetsJSONField, "notes":
		return true
	default:
		return false
	}
}

func normalizeSecurityGroupJSON(value any, field string) any {
	switch value := value.(type) {
	case map[string]any:
		normalized := make(map[string]any, len(value))
		for key, child := range value {
			if key == securityGroupTargetsJSONField {
				if targets, ok := child.([]any); ok && len(targets) == 0 {
					continue
				}
			}
			if (key == "ports" || key == "notes") && child == "" {
				continue
			}
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
		"reordered non-set array": {
			method: http.MethodPost,
			url:    "https://api.example.test/security_groups",
			body:   `{"properties":{"tags":["b","a"]}}`,
			recorded: cassette.Request{
				Method: http.MethodPost,
				URL:    "https://api.example.test/security_groups",
				Body:   `{"properties":{"tags":["a","b"]}}`,
			},
			want:         false,
			wantReadable: true,
		},
		"matching non-JSON bodies": {
			method: http.MethodPost,
			url:    "https://api.example.test/security_groups",
			body:   "plain body",
			recorded: cassette.Request{
				Method: http.MethodPost,
				URL:    "https://api.example.test/security_groups",
				Body:   "plain body",
			},
			want:         true,
			wantReadable: true,
		},
		"different non-JSON bodies": {
			method: http.MethodPost,
			url:    "https://api.example.test/security_groups",
			body:   "actual body",
			recorded: cassette.Request{
				Method: http.MethodPost,
				URL:    "https://api.example.test/security_groups",
				Body:   "recorded body",
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
				assert.Equal(t, test.body, string(body))
			}
		})
	}
}

func TestSecurityGroupJSONBodiesEqualRejectsUnknownActualProperties(t *testing.T) {
	t.Parallel()

	assert.False(t, securityGroupJSONBodiesEqual(
		[]byte(`{"properties":{"name":"web","unexpected":"regression"}}`),
		[]byte(`{"properties":{"name":"web"}}`),
	))
}

func TestSecurityGroupJSONBodiesEqualTreatsOnlyAllowAsLegacyOmission(t *testing.T) {
	t.Parallel()

	recorded := []byte(`{"properties":{"protocol":"TCP","targets":["all:ipv4"]}}`)
	allow := []byte(`{"properties":{"action":"allow","protocol":"TCP","targets":["all:ipv4"]}}`)
	deny := []byte(`{"properties":{"action":"deny","protocol":"TCP","targets":["all:ipv4"]}}`)

	assert.True(t, securityGroupJSONBodiesEqual(allow, recorded))
	assert.False(t, securityGroupJSONBodiesEqual(deny, recorded))
	assert.False(t, securityGroupJSONBodiesEqual(recorded, allow))
}

func TestOrderedSecurityGroupCassetteTransportGETFallbackRequiresMatchingBody(t *testing.T) {
	t.Parallel()

	const requestURL = "https://api.example.test/security_groups"
	interaction := &cassette.Interaction{
		Request: cassette.Request{
			Method: http.MethodGet,
			URL:    requestURL,
			Body:   `{"properties":{"name":"recorded"}}`,
		},
		Response: cassette.Response{
			Body: `{}`, Status: "200 OK", Code: http.StatusOK,
		},
	}
	key := securityGroupCassetteRequestKey(http.MethodGet, requestURL, nil)
	transport := &orderedSecurityGroupCassetteTransport{
		interactions: map[string][]*cassette.Interaction{key: {interaction}},
		next:         map[string]int{key: 1},
	}
	request, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, requestURL,
		strings.NewReader(`{"properties":{"name":"actual"}}`),
	)
	require.NoError(t, err)

	response, err := transport.RoundTrip(request)
	if response != nil {
		require.NoError(t, response.Body.Close())
	}

	assert.Nil(t, response)
	assert.ErrorIs(t, err, cassette.ErrInteractionNotFound)
}

func TestOrderedSecurityGroupCassetteTransportMatchesDistinctCreatesOutOfOrder(t *testing.T) {
	t.Parallel()

	const requestURL = "https://api.example.test/organizations/organization/security_groups"
	webBody := `{"organization":{"sub_domain":"test"},"properties":{"name":"web","associations":[]}}`
	dynamicBody := `{"organization":{"sub_domain":"test"},"properties":{"name":"dynamic","associations":[]}}`
	webInteraction := &cassette.Interaction{
		Request: cassette.Request{Method: http.MethodPost, URL: requestURL, Body: webBody},
		Response: cassette.Response{
			Body: `{"result":"web"}`, Status: "200 OK", Code: http.StatusOK,
		},
	}
	dynamicInteraction := &cassette.Interaction{
		Request: cassette.Request{Method: http.MethodPost, URL: requestURL, Body: dynamicBody},
		Response: cassette.Response{
			Body: `{"result":"dynamic"}`, Status: "200 OK", Code: http.StatusOK,
		},
	}
	key := securityGroupCassetteRequestKey(http.MethodPost, requestURL, nil)
	transport := &orderedSecurityGroupCassetteTransport{
		interactions: map[string][]*cassette.Interaction{
			key: {webInteraction, dynamicInteraction},
		},
		next: map[string]int{},
	}

	for _, test := range []struct {
		requestBody  string
		responseBody string
	}{
		{requestBody: dynamicBody, responseBody: `{"result":"dynamic"}`},
		{requestBody: webBody, responseBody: `{"result":"web"}`},
	} {
		request, err := http.NewRequestWithContext(
			context.Background(), http.MethodPost, requestURL,
			strings.NewReader(test.requestBody),
		)
		require.NoError(t, err)

		response, err := transport.RoundTrip(request)
		require.NoError(t, err)
		responseBody, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		assert.JSONEq(t, test.responseBody, string(responseBody))
	}
}

func TestOrderedSecurityGroupCassetteTransportCompatibilityCreateSupportsRead(t *testing.T) {
	t.Parallel()

	const (
		createURL  = "https://api.example.test/organizations/organization/security_groups"
		readURL    = "https://api.example.test/security_groups/security_group?security_group%5Bid%5D=sg_1"
		actualBody = `{"organization":{"sub_domain":"test"},"properties":{` +
			`"name":"web","associations":["vmgrp_2","vmgrp_1"]}}`
		recordedBody = `{"organization":{"sub_domain":"test"},"properties":{` +
			`"name":"web","associations":["vmgrp_1","vmgrp_2"],"allow_all_inbound":false}}`
		responseBody = `{"security_group":{"id":"sg_1","name":"web",` +
			`"associations":["vmgrp_2","vmgrp_1"],"allow_all_inbound":false}}`
	)
	interaction := &cassette.Interaction{
		Request: cassette.Request{
			Method: http.MethodPost, URL: createURL, Body: recordedBody,
		},
		Response: cassette.Response{
			Body: responseBody, Status: "200 OK", Code: http.StatusOK,
		},
	}
	key := securityGroupCassetteRequestKey(http.MethodPost, createURL, nil)
	transport := &orderedSecurityGroupCassetteTransport{
		interactions:   map[string][]*cassette.Interaction{key: {interaction}},
		next:           map[string]int{},
		groupSnapshots: map[string]securityGroupReplaySnapshot{},
	}

	createRequest, err := http.NewRequestWithContext(
		context.Background(), http.MethodPost, createURL, strings.NewReader(actualBody),
	)
	require.NoError(t, err)
	createResponse, err := transport.RoundTrip(createRequest)
	require.NoError(t, err)
	require.NoError(t, createResponse.Body.Close())

	readRequest, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, readURL, nil,
	)
	require.NoError(t, err)
	readResponse, err := transport.RoundTrip(readRequest)
	require.NoError(t, err)
	readBody, err := io.ReadAll(readResponse.Body)
	require.NoError(t, err)
	require.NoError(t, readResponse.Body.Close())
	assert.JSONEq(t, responseBody, string(readBody))
}

func TestOrderedSecurityGroupCassetteTransportCreateFallbackRejectsUnknownProperties(t *testing.T) {
	t.Parallel()

	const requestURL = "https://api.example.test/organizations/_/security_groups"
	interaction := &cassette.Interaction{
		Request: cassette.Request{
			Method: http.MethodPost,
			URL:    requestURL,
			Body:   `{"organization":{"sub_domain":"test"},"properties":{"name":"web","associations":[]}}`,
		},
		Response: cassette.Response{
			Body: `{}`, Status: "200 OK", Code: http.StatusOK,
		},
	}
	key := securityGroupCassetteRequestKey(http.MethodPost, requestURL, nil)
	transport := &orderedSecurityGroupCassetteTransport{
		interactions: map[string][]*cassette.Interaction{key: {interaction}},
		next:         map[string]int{},
	}
	request, err := http.NewRequestWithContext(
		context.Background(), http.MethodPost, requestURL,
		strings.NewReader(
			`{"organization":{"sub_domain":"test"},`+
				`"properties":{"name":"web","associations":[],"unexpected":"regression"}}`,
		),
	)
	require.NoError(t, err)

	response, err := transport.RoundTrip(request)
	if response != nil {
		require.NoError(t, response.Body.Close())
	}

	assert.Nil(t, response)
	assert.ErrorIs(t, err, cassette.ErrInteractionNotFound)
}

func TestOrderedSecurityGroupCassetteTransportCreateFallbackRejectsMismatchedAllowAll(t *testing.T) {
	t.Parallel()

	const (
		requestURL   = "https://api.example.test/organizations/_/security_groups"
		recordedBody = `{"organization":{"sub_domain":"test"},"properties":{` +
			`"name":"web","associations":[],` +
			`"allow_all_inbound":false,"allow_all_outbound":false}}`
	)
	tests := map[string]string{
		"inbound": `{"organization":{"sub_domain":"test"},"properties":{` +
			`"name":"web","associations":[],` +
			`"allow_all_inbound":true,"allow_all_outbound":false}}`,
		"outbound": `{"organization":{"sub_domain":"test"},"properties":{` +
			`"name":"web","associations":[],` +
			`"allow_all_inbound":false,"allow_all_outbound":true}}`,
	}
	for name, actualBody := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			interaction := &cassette.Interaction{
				Request: cassette.Request{
					Method: http.MethodPost,
					URL:    requestURL,
					Body:   recordedBody,
				},
				Response: cassette.Response{
					Body: `{}`, Status: "200 OK", Code: http.StatusOK,
				},
			}
			key := securityGroupCassetteRequestKey(http.MethodPost, requestURL, nil)
			transport := &orderedSecurityGroupCassetteTransport{
				interactions: map[string][]*cassette.Interaction{key: {interaction}},
				next:         map[string]int{},
			}
			request, err := http.NewRequestWithContext(
				context.Background(), http.MethodPost, requestURL,
				strings.NewReader(actualBody),
			)
			require.NoError(t, err)

			response, err := transport.RoundTrip(request)
			if response != nil {
				require.NoError(t, response.Body.Close())
			}

			assert.Nil(t, response)
			assert.ErrorIs(t, err, cassette.ErrInteractionNotFound)
		})
	}
}
