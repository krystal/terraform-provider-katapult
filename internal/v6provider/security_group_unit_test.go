package v6provider

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	schemavalidator "github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:lll // Side-by-side canonical fixtures make normalization differences obvious.
func TestSecurityGroupRuleFingerprintNormalizesProtocolAndTargets(t *testing.T) {
	t.Parallel()

	a := canonicalSecurityGroupRule{Direction: "inbound", Protocol: "tcp", Ports: "22", Targets: []string{"all:ipv6", "all:ipv4"}, Notes: "SSH"}
	b := canonicalSecurityGroupRule{Direction: "INBOUND", Protocol: "TCP", Ports: "22", Targets: []string{"all:ipv4", "all:ipv6"}, Notes: "SSH"}

	assert.Equal(t, a.fingerprint(), b.fingerprint())
}

func TestTransferSecurityGroupRuleIDsPreservesDuplicateMultiset(t *testing.T) {
	t.Parallel()

	prior := []canonicalSecurityGroupRule{
		{ID: "rule-1", Direction: "inbound", Protocol: "TCP", Targets: []string{"all:ipv4"}},
		{ID: "rule-2", Direction: "inbound", Protocol: "TCP", Targets: []string{"all:ipv4"}},
	}
	planned := []canonicalSecurityGroupRule{
		{Direction: "inbound", Protocol: "TCP", Targets: []string{"all:ipv4"}},
		{Direction: "inbound", Protocol: "TCP", Targets: []string{"all:ipv4"}},
	}

	result := transferSecurityGroupRuleIDs(prior, planned, false)
	require.Len(t, result, 2)
	assert.Equal(t, "rule-1", result[0].ID)
	assert.Equal(t, "rule-2", result[1].ID)
}

func TestTransferSecurityGroupRuleIDsPrefersExplicitID(t *testing.T) {
	t.Parallel()

	prior := []canonicalSecurityGroupRule{
		{ID: "rule-1", Direction: "outbound", Protocol: "UDP", Targets: []string{"all:ipv4"}},
		{ID: "rule-2", Direction: "outbound", Protocol: "UDP", Targets: []string{"all:ipv4"}},
	}
	planned := []canonicalSecurityGroupRule{
		{ID: "rule-2", Direction: "outbound", Protocol: "UDP", Targets: []string{"all:ipv4"}},
		{Direction: "outbound", Protocol: "UDP", Targets: []string{"all:ipv4"}},
	}

	result := transferSecurityGroupRuleIDs(prior, planned, false)
	assert.Equal(t, "rule-2", result[0].ID)
	assert.Equal(t, "rule-1", result[1].ID)
}

func TestTransferSecurityGroupRuleIDsPairsSingleResidualMaterialEdit(t *testing.T) {
	t.Parallel()

	prior := []canonicalSecurityGroupRule{
		{ID: "ssh", Direction: "inbound", Protocol: "TCP", Ports: "22", Targets: []string{"all:ipv4"}},
		{ID: "dns", Direction: "inbound", Protocol: "UDP", Ports: "53", Targets: []string{"all:ipv4"}},
	}
	planned := []canonicalSecurityGroupRule{
		{Direction: "inbound", Protocol: "UDP", Ports: "53", Targets: []string{"all:ipv4"}},
		{Direction: "inbound", Protocol: "TCP", Ports: "2222", Targets: []string{"all:ipv4"}},
	}

	result := transferSecurityGroupRuleIDs(prior, planned, true)
	require.Len(t, result, 2)
	assert.Equal(t, "dns", result[0].ID)
	assert.Equal(t, "ssh", result[1].ID)
	reconciliation := classifySecurityGroupRuleReconciliation(prior, result)
	assert.Empty(t, reconciliation.CreateIndexes)
	assert.Equal(t, []int{1}, reconciliation.UpdateIndexes)
	assert.Empty(t, reconciliation.Delete)
}

//nolint:lll // Compact fixtures make reconciliation cases directly comparable.
func TestClassifySecurityGroupRuleReconciliation(t *testing.T) {
	t.Parallel()

	ssh := canonicalSecurityGroupRule{ID: "ssh", Direction: "inbound", Protocol: "TCP", Ports: "22", Targets: []string{"all:ipv4"}, Notes: "SSH"}
	dns := canonicalSecurityGroupRule{ID: "dns", Direction: "inbound", Protocol: "UDP", Ports: "53", Targets: []string{"all:ipv4"}, Notes: "DNS"}
	http := canonicalSecurityGroupRule{Direction: "inbound", Protocol: "TCP", Ports: "80,443", Targets: []string{"all:ipv4"}, Notes: "HTTP"}
	changedDNS := dns
	changedDNS.Targets = []string{"all:ipv4", "all:ipv6"}

	tests := []struct {
		name                   string
		prior, desired         []canonicalSecurityGroupRule
		create, update, delete int
	}{
		{name: "create first", desired: []canonicalSecurityGroupRule{http}, create: 1},
		{name: "add and reorder", prior: []canonicalSecurityGroupRule{ssh, dns}, desired: []canonicalSecurityGroupRule{{Direction: dns.Direction, Protocol: dns.Protocol, Ports: dns.Ports, Targets: dns.Targets, Notes: dns.Notes}, http, {Direction: ssh.Direction, Protocol: ssh.Protocol, Ports: ssh.Ports, Targets: ssh.Targets, Notes: ssh.Notes}}, create: 1},
		{name: "update", prior: []canonicalSecurityGroupRule{ssh, dns}, desired: []canonicalSecurityGroupRule{ssh, changedDNS}, update: 1},
		{name: "delete", prior: []canonicalSecurityGroupRule{ssh, dns}, desired: []canonicalSecurityGroupRule{ssh}, delete: 1},
		{name: "create update delete", prior: []canonicalSecurityGroupRule{ssh, dns}, desired: []canonicalSecurityGroupRule{changedDNS, http}, create: 1, update: 1, delete: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			result := classifySecurityGroupRuleReconciliation(test.prior, test.desired)
			assert.Len(t, result.CreateIndexes, test.create)
			assert.Len(t, result.UpdateIndexes, test.update)
			assert.Len(t, result.Delete, test.delete)
		})
	}
}

//nolint:lll // Full rules keep duplicate pairing behavior explicit.
func TestClassifySecurityGroupRuleReconciliationReorderAndDuplicatesAreEmpty(t *testing.T) {
	t.Parallel()

	prior := []canonicalSecurityGroupRule{
		{ID: "ssh", Direction: "inbound", Protocol: "TCP", Ports: "22", Targets: []string{"all:ipv4", "all:ipv6"}, Notes: "SSH"},
		{ID: "dup-1", Direction: "inbound", Protocol: "ICMP", Targets: []string{"all:ipv4"}},
		{ID: "dup-2", Direction: "inbound", Protocol: "ICMP", Targets: []string{"all:ipv4"}},
	}
	desired := []canonicalSecurityGroupRule{
		{Direction: "inbound", Protocol: "icmp", Targets: []string{"all:ipv4"}},
		{Direction: "inbound", Protocol: "TCP", Ports: "22", Targets: []string{"all:ipv6", "all:ipv4"}, Notes: "SSH"},
		{Direction: "inbound", Protocol: "ICMP", Targets: []string{"all:ipv4"}},
	}

	result := classifySecurityGroupRuleReconciliation(prior, desired)
	assert.Empty(t, result.CreateIndexes)
	assert.Empty(t, result.UpdateIndexes)
	assert.Empty(t, result.Delete)
	require.Len(t, result.Desired, 3)
	assert.Equal(t, []string{"dup-1", "ssh", "dup-2"}, []string{
		result.Desired[0].ID,
		result.Desired[1].ID,
		result.Desired[2].ID,
	})
}

func TestPreserveSecurityGroupRuleStateOrder(t *testing.T) {
	t.Parallel()

	prior := []canonicalSecurityGroupRule{{ID: "second"}, {ID: "first"}}
	remote := []canonicalSecurityGroupRule{{ID: "first"}, {ID: "new"}, {ID: "second"}}
	ordered := preserveSecurityGroupRuleStateOrder(prior, remote)
	require.Len(t, ordered, 3)
	assert.Equal(t, []string{"second", "first", "new"}, []string{
		ordered[0].ID, ordered[1].ID, ordered[2].ID,
	})
}

func TestSecurityGroupSchemaSupportsBothRuleRepresentations(t *testing.T) {
	t.Parallel()

	var response resource.SchemaResponse
	(&SecurityGroupResource{}).Schema(context.Background(), resource.SchemaRequest{}, &response)
	require.False(t, response.Diagnostics.HasError())
	assert.Contains(t, response.Schema.Attributes, "inbound_rules")
	assert.Contains(t, response.Schema.Attributes, "outbound_rules")
	assert.Contains(t, response.Schema.Blocks, "inbound_rule")
	assert.Contains(t, response.Schema.Blocks, "outbound_rule")
}

func TestSecurityGroupStringSetSchemasRejectEmptyAndNullValues(t *testing.T) {
	t.Parallel()

	var groupResponse, ruleResponse resource.SchemaResponse
	(&SecurityGroupResource{}).Schema(context.Background(), resource.SchemaRequest{}, &groupResponse)
	(&SecurityGroupRuleResource{}).Schema(context.Background(), resource.SchemaRequest{}, &ruleResponse)
	require.False(t, groupResponse.Diagnostics.HasError(), groupResponse.Diagnostics.Errors())
	require.False(t, ruleResponse.Diagnostics.HasError(), ruleResponse.Diagnostics.Errors())

	plural := groupResponse.Schema.Attributes["inbound_rules"].(schema.ListNestedAttribute)
	block := groupResponse.Schema.Blocks["inbound_rule"].(schema.ListNestedBlock)
	attributes := map[string]schema.SetAttribute{
		"associations":           groupResponse.Schema.Attributes["associations"].(schema.SetAttribute),
		"plural rule targets":    plural.NestedObject.Attributes["targets"].(schema.SetAttribute),
		"deprecated rule target": block.NestedObject.Attributes["targets"].(schema.SetAttribute),
		"standalone rule target": ruleResponse.Schema.Attributes["targets"].(schema.SetAttribute),
	}
	values := map[string]types.Set{
		"empty string": types.SetValueMust(types.StringType, []attr.Value{types.StringValue("")}),
		"null element": types.SetValueMust(types.StringType, []attr.Value{types.StringNull()}),
	}

	for attributeName, attribute := range attributes {
		for valueName, value := range values {
			t.Run(attributeName+" "+valueName, func(t *testing.T) {
				t.Parallel()
				diagnostics := runSecurityGroupSetValidators(attribute, value)
				require.True(t, diagnostics.HasError(), diagnostics)
				require.Len(t, diagnostics.Errors(), 1, diagnostics)
			})
		}
	}
}

func TestSecurityGroupRuleValidateConfigLeavesTargetsToSchemaValidators(t *testing.T) {
	t.Parallel()

	model := SecurityGroupRuleResourceModel{
		ID:              types.StringNull(),
		SecurityGroupID: types.StringValue("security_group_test"),
		Direction:       types.StringValue("inbound"),
		Protocol:        caseInsensitiveStringValueOf("TCP"),
		Ports:           types.StringValue("22"),
		Targets:         types.SetValueMust(types.StringType, []attr.Value{types.StringValue("")}),
		Notes:           types.StringValue(""),
	}
	state := securityGroupRuleTestState(t, model)
	response := resource.ValidateConfigResponse{}
	(&SecurityGroupRuleResource{}).ValidateConfig(context.Background(), resource.ValidateConfigRequest{
		Config: tfsdk.Config(state),
	}, &response)

	require.False(t, response.Diagnostics.HasError(), response.Diagnostics.Errors())
}

func TestSecurityGroupRuleCollectionClassification(t *testing.T) {
	t.Parallel()

	empty := types.ListValueMust(securityGroupRuleObjectType(), nil)
	nonEmpty := securityGroupTestRuleList(t, []canonicalSecurityGroupRule{{
		Direction: "inbound", Protocol: "TCP", Targets: []string{"all:ipv4"},
	}}, false)
	tests := []struct {
		name       string
		value      types.List
		configured bool
		hasValues  bool
	}{
		{name: "omitted", value: types.ListNull(securityGroupRuleObjectType())},
		{name: "explicit null", value: types.ListNull(securityGroupRuleObjectType())},
		{name: "unknown", value: types.ListUnknown(securityGroupRuleObjectType())},
		{name: "explicit empty", value: empty, configured: true},
		{name: "configured values", value: nonEmpty, configured: true, hasValues: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, test.configured, knownCollectionConfigured(test.value))
			assert.Equal(t, test.hasValues, knownCollectionHasValues(test.value))
		})
	}
}

func TestSecurityGroupValidateConfigRuleRepresentationMatrix(t *testing.T) {
	t.Parallel()

	null := types.ListNull(securityGroupRuleObjectType())
	empty := types.ListValueMust(securityGroupRuleObjectType(), nil)
	unknown := types.ListUnknown(securityGroupRuleObjectType())
	inbound := securityGroupTestRuleList(t, []canonicalSecurityGroupRule{{
		Direction: "inbound", Protocol: "TCP", Ports: "22", Targets: []string{"all:ipv4"},
	}}, false)
	outbound := securityGroupTestRuleList(t, []canonicalSecurityGroupRule{{
		Direction: "outbound", Protocol: "UDP", Ports: "53", Targets: []string{"all:ipv4"},
	}}, false)
	tests := []struct {
		name       string
		configure  func(*SecurityGroupResourceModel)
		wantDetail string
	}{
		{
			name: "same direction attribute and block",
			configure: func(model *SecurityGroupResourceModel) {
				model.InboundRules, model.InboundRule = empty, empty
			},
			wantDetail: "either the plural rules attribute or the deprecated singular rule blocks",
		},
		{
			name: "inbound allow all with plural rules",
			configure: func(model *SecurityGroupResourceModel) {
				model.AllowAllInbound, model.InboundRules = types.BoolValue(true), inbound
			},
			wantDetail: "Allow-all cannot be enabled",
		},
		{
			name: "outbound allow all with plural rules",
			configure: func(model *SecurityGroupResourceModel) {
				model.AllowAllOutbound, model.OutboundRules = types.BoolValue(true), outbound
			},
			wantDetail: "Allow-all cannot be enabled",
		},
		{
			name: "external rules with plural values",
			configure: func(model *SecurityGroupResourceModel) {
				model.ExternalRules, model.InboundRules = types.BoolValue(true), inbound
			},
			wantDetail: "external_rules cannot be enabled",
		},
		{
			name: "external rules with explicit empty plural collection",
			configure: func(model *SecurityGroupResourceModel) {
				model.ExternalRules, model.OutboundRules = types.BoolValue(true), empty
			},
			wantDetail: "external_rules cannot be enabled",
		},
		{
			name: "unknown plural representation defers block conflict",
			configure: func(model *SecurityGroupResourceModel) {
				model.InboundRules, model.InboundRule = unknown, empty
			},
		},
		{
			name: "unknown allow all defers plural conflict",
			configure: func(model *SecurityGroupResourceModel) {
				model.AllowAllInbound, model.InboundRules = types.BoolUnknown(), inbound
			},
		},
		{
			name: "unknown external rules defers plural conflict",
			configure: func(model *SecurityGroupResourceModel) {
				model.ExternalRules, model.InboundRules = types.BoolUnknown(), inbound
			},
		},
		{
			name: "known external rules defers unknown plural collection",
			configure: func(model *SecurityGroupResourceModel) {
				model.ExternalRules, model.InboundRules = types.BoolValue(true), unknown
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			model := securityGroupTestModel()
			model.InboundRules, model.OutboundRules = null, null
			model.InboundRule, model.OutboundRule = null, null
			test.configure(&model)
			response := runSecurityGroupValidateConfig(t, model)
			if test.wantDetail == "" {
				require.False(t, response.Diagnostics.HasError(), response.Diagnostics.Errors())
				return
			}
			require.True(t, response.Diagnostics.HasError())
			require.Contains(t, response.Diagnostics.Errors()[0].Detail(), test.wantDetail)
		})
	}
}

func TestSecurityGroupApplyRejectsResolvedInvalidConfigurationBeforeAPI(t *testing.T) {
	t.Parallel()

	null := types.ListNull(securityGroupRuleObjectType())
	empty := types.ListValueMust(securityGroupRuleObjectType(), nil)
	inbound := securityGroupTestRuleList(t, []canonicalSecurityGroupRule{{
		Direction: "inbound", Protocol: "TCP", Ports: "22", Targets: []string{"all:ipv4"},
	}}, false)

	for _, test := range []struct {
		name      string
		configure func(*SecurityGroupResourceModel, *SecurityGroupResourceModel)
		apply     func(*SecurityGroupResource, tfsdk.State, tfsdk.State, tfsdk.State) diag.Diagnostics
	}{
		{
			name: "create external rules with inline rules",
			configure: func(plan, config *SecurityGroupResourceModel) {
				plan.ExternalRules, plan.InboundRules = types.BoolValue(true), inbound
				config.ExternalRules, config.InboundRules = types.BoolUnknown(), inbound
			},
			apply: func(resourceUnderTest *SecurityGroupResource, _, plan, config tfsdk.State) diag.Diagnostics {
				response := resource.CreateResponse{State: plan}
				resourceUnderTest.Create(context.Background(), resource.CreateRequest{
					Config: tfsdk.Config(config), Plan: tfsdk.Plan(plan),
				}, &response)
				return response.Diagnostics
			},
		},
		{
			name: "update dual inbound representations",
			configure: func(plan, config *SecurityGroupResourceModel) {
				plan.Name, config.Name = types.StringValue("Changed"), types.StringValue("Changed")
				plan.InboundRules, plan.InboundRule = empty, empty
				config.InboundRules = empty
				config.InboundRule = types.ListUnknown(securityGroupRuleObjectType())
			},
			apply: func(resourceUnderTest *SecurityGroupResource, state, plan, config tfsdk.State) diag.Diagnostics {
				response := resource.UpdateResponse{State: plan}
				resourceUnderTest.Update(context.Background(), resource.UpdateRequest{
					Config: tfsdk.Config(config), State: state, Plan: tfsdk.Plan(plan),
				}, &response)
				return response.Diagnostics
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var requests atomic.Int32
			client := newVirtualMachineTestClient(t, func(writer http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				http.Error(writer, "unexpected request", http.StatusInternalServerError)
			})
			resourceUnderTest := &SecurityGroupResource{M: &Meta{
				Core: client, confOrganization: "terraform-acc-test", testMode: true,
			}}
			stateModel := securityGroupTestModel()
			planModel := securityGroupTestModel()
			configModel := securityGroupTestModel()
			planModel.InboundRules, planModel.OutboundRules = null, null
			planModel.InboundRule, planModel.OutboundRule = null, null
			configModel.InboundRules, configModel.OutboundRules = null, null
			configModel.InboundRule, configModel.OutboundRule = null, null
			test.configure(&planModel, &configModel)

			diagnostics := test.apply(
				resourceUnderTest,
				securityGroupTestState(t, stateModel),
				securityGroupTestState(t, planModel),
				securityGroupTestState(t, configModel),
			)

			require.True(t, diagnostics.HasError(), diagnostics)
			assert.Zero(t, requests.Load(), "invalid apply reached the API")
		})
	}
}

func TestSecurityGroupApplyUnknownDecisionValidation(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		configure func(*SecurityGroupResourceModel, *SecurityGroupResourceModel)
		wantError bool
	}{
		{
			name: "configured external rules remains unknown",
			configure: func(plan, config *SecurityGroupResourceModel) {
				plan.ExternalRules, config.ExternalRules = types.BoolUnknown(), types.BoolUnknown()
			},
			wantError: true,
		},
		{
			name: "configured plural rules remain unknown",
			configure: func(plan, config *SecurityGroupResourceModel) {
				unknown := types.ListUnknown(securityGroupRuleObjectType())
				plan.InboundRules, config.InboundRules = unknown, unknown
			},
			wantError: true,
		},
		{
			name: "configured nested rule field remains unknown",
			configure: func(plan, config *SecurityGroupResourceModel) {
				plan.InboundRules = securityGroupTestUnknownRuleList(t, true)
				config.InboundRules = securityGroupTestUnknownRuleList(t, false)
			},
			wantError: true,
		},
		{
			name: "omitted optional computed values may remain unknown",
			configure: func(plan, config *SecurityGroupResourceModel) {
				plan.ExternalRules = types.BoolUnknown()
				plan.InboundRules = types.ListUnknown(securityGroupRuleObjectType())
				config.ExternalRules = types.BoolNull()
				config.InboundRules = types.ListNull(securityGroupRuleObjectType())
			},
		},
		{
			name: "configured dynamic collection resolved to null",
			configure: func(plan, config *SecurityGroupResourceModel) {
				plan.InboundRules = types.ListNull(securityGroupRuleObjectType())
				config.InboundRules = types.ListUnknown(securityGroupRuleObjectType())
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			plan, config := securityGroupTestModel(), securityGroupTestModel()
			test.configure(&plan, &config)
			diagnostics := validateSecurityGroupApplyUnknowns(plan, config)
			assert.Equal(t, test.wantError, diagnostics.HasError(), diagnostics)
		})
	}
}

func TestSecurityGroupModifyPlanMigratesRuleRepresentationsWithoutReconciliation(t *testing.T) {
	t.Parallel()

	rule := canonicalSecurityGroupRule{
		ID: "rule-1", Direction: "inbound", Protocol: "TCP", Ports: "22",
		Targets: []string{"all:ipv4", "all:ipv6"}, Notes: "SSH",
	}
	known := securityGroupTestRuleList(t, []canonicalSecurityGroupRule{rule}, false)
	planned := securityGroupTestRuleList(t, []canonicalSecurityGroupRule{{
		Direction: "inbound", Protocol: "tcp", Ports: "22",
		Targets: []string{"all:ipv6", "all:ipv4"}, Notes: "SSH",
	}}, true)
	empty := types.ListValueMust(securityGroupRuleObjectType(), nil)
	null := types.ListNull(securityGroupRuleObjectType())

	for _, test := range []struct {
		name       string
		stateBlock bool
	}{
		{name: "blocks to attributes", stateBlock: true},
		{name: "attributes to blocks"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			state, plan := securityGroupTestModel(), securityGroupTestModel()
			state.OutboundRules, plan.OutboundRules = empty, empty
			state.OutboundRule, plan.OutboundRule = null, null
			if test.stateBlock {
				state.InboundRule, state.InboundRules = known, null
				plan.InboundRules, plan.InboundRule = planned, null
			} else {
				state.InboundRules, state.InboundRule = known, null
				plan.InboundRule, plan.InboundRules = planned, null
			}
			response := runSecurityGroupModifyPlan(t, state, plan, plan)
			require.False(t, response.Diagnostics.HasError(), response.Diagnostics.Errors())
			var got SecurityGroupResourceModel
			diags := response.Plan.Get(context.Background(), &got)
			require.False(t, diags.HasError(), diags.Errors())
			selected, legacy, err := selectedRules(context.Background(), got, securityGroupDirectionInbound)
			require.NoError(t, err)
			require.Equal(t, !test.stateBlock, legacy)
			require.Len(t, selected, 1)
			assert.Equal(t, "rule-1", selected[0].ID)

			reconciliation := classifySecurityGroupRuleReconciliation([]canonicalSecurityGroupRule{rule}, selected)
			assert.Empty(t, reconciliation.CreateIndexes)
			assert.Empty(t, reconciliation.UpdateIndexes)
			assert.Empty(t, reconciliation.Delete)
		})
	}
}

//nolint:lll // Both representation directions must retain the edited rule identity.
func TestSecurityGroupModifyPlanMigratesRepresentationsWithMaterialEdit(t *testing.T) {
	t.Parallel()

	ssh := canonicalSecurityGroupRule{ID: "ssh", Direction: "inbound", Protocol: "TCP", Ports: "22", Targets: []string{"all:ipv4"}, Notes: "SSH"}
	dns := canonicalSecurityGroupRule{ID: "dns", Direction: "inbound", Protocol: "UDP", Ports: "53", Targets: []string{"all:ipv4"}, Notes: "DNS"}
	changedSSH := ssh
	changedSSH.Ports = "2222"
	prior := []canonicalSecurityGroupRule{ssh, dns}
	planned := []canonicalSecurityGroupRule{
		{Direction: dns.Direction, Protocol: dns.Protocol, Ports: dns.Ports, Targets: dns.Targets, Notes: dns.Notes},
		{Direction: changedSSH.Direction, Protocol: changedSSH.Protocol, Ports: changedSSH.Ports, Targets: changedSSH.Targets, Notes: changedSSH.Notes},
	}
	known := securityGroupTestRuleList(t, prior, false)
	destination := securityGroupTestRuleList(t, planned, true)
	empty := types.ListValueMust(securityGroupRuleObjectType(), nil)
	null := types.ListNull(securityGroupRuleObjectType())

	for _, stateBlock := range []bool{true, false} {
		name := "attributes to blocks"
		if stateBlock {
			name = "blocks to attributes"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			state, plan := securityGroupTestModel(), securityGroupTestModel()
			state.OutboundRules, plan.OutboundRules = empty, empty
			state.OutboundRule, plan.OutboundRule = null, null
			if stateBlock {
				state.InboundRule, state.InboundRules = known, null
				plan.InboundRules, plan.InboundRule = destination, null
			} else {
				state.InboundRules, state.InboundRule = known, null
				plan.InboundRule, plan.InboundRules = destination, null
			}

			response := runSecurityGroupModifyPlan(t, state, plan, plan)
			require.False(t, response.Diagnostics.HasError(), response.Diagnostics.Errors())
			var got SecurityGroupResourceModel
			diags := response.Plan.Get(context.Background(), &got)
			require.False(t, diags.HasError(), diags.Errors())
			desired, legacy, err := selectedRules(context.Background(), got, securityGroupDirectionInbound)
			require.NoError(t, err)
			assert.Equal(t, !stateBlock, legacy)
			require.Len(t, desired, 2)
			assert.Equal(t, []string{"dns", "ssh"}, []string{desired[0].ID, desired[1].ID})
			reconciliation := classifySecurityGroupRuleReconciliation(prior, desired)
			assert.Empty(t, reconciliation.CreateIndexes)
			assert.Equal(t, []int{1}, reconciliation.UpdateIndexes)
			assert.Empty(t, reconciliation.Delete)
		})
	}
}

func TestSecurityGroupModifyPlanDefersUnknownRuleRepresentations(t *testing.T) {
	t.Parallel()

	rule := canonicalSecurityGroupRule{
		ID: "rule-1", Direction: "inbound", Protocol: "TCP", Ports: "22",
		Targets: []string{"all:ipv4"}, Notes: "SSH",
	}
	known := securityGroupTestRuleList(t, []canonicalSecurityGroupRule{rule}, false)
	empty := types.ListValueMust(securityGroupRuleObjectType(), nil)
	null := types.ListNull(securityGroupRuleObjectType())
	unknown := types.ListUnknown(securityGroupRuleObjectType())

	for _, test := range []struct {
		name         string
		stateLegacy  bool
		unknownBlock bool
	}{
		{name: "dependency driven plural attribute preserves prior blocks", stateLegacy: true},
		{name: "dynamic legacy blocks preserve prior attributes", unknownBlock: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			state, plan, config := securityGroupTestModel(), securityGroupTestModel(), securityGroupTestModel()
			state.OutboundRules, plan.OutboundRules, config.OutboundRules = empty, empty, empty
			state.OutboundRule, plan.OutboundRule, config.OutboundRule = null, null, null
			if test.stateLegacy {
				state.InboundRule, state.InboundRules = known, null
				plan.InboundRules, plan.InboundRule = unknown, null
				config.InboundRules, config.InboundRule = unknown, null
			} else {
				state.InboundRules, state.InboundRule = known, null
				plan.InboundRule, plan.InboundRules = unknown, null
				config.InboundRule, config.InboundRules = unknown, null
			}

			response := runSecurityGroupModifyPlan(t, state, plan, config)
			require.False(t, response.Diagnostics.HasError(), response.Diagnostics.Errors())
			var got SecurityGroupResourceModel
			diags := response.Plan.Get(context.Background(), &got)
			require.False(t, diags.HasError(), diags.Errors())
			if test.unknownBlock {
				assert.True(t, got.InboundRule.IsUnknown())
				assert.True(t, got.InboundRules.IsNull())
			} else {
				assert.True(t, got.InboundRules.IsUnknown())
				assert.True(t, got.InboundRule.IsNull())
			}
		})
	}
}

func TestSecurityGroupModifyPlanPreservesNestedUnknownRuleFields(t *testing.T) {
	t.Parallel()

	priorRule := canonicalSecurityGroupRule{
		ID: "rule-1", Direction: "inbound", Protocol: "TCP", Ports: "22",
		Targets: []string{"all:ipv4"}, Notes: "SSH",
	}
	prior := securityGroupTestRuleList(t, []canonicalSecurityGroupRule{priorRule}, false)
	empty := types.ListValueMust(securityGroupRuleObjectType(), nil)
	null := types.ListNull(securityGroupRuleObjectType())

	for _, legacy := range []bool{false, true} {
		name := "plural attributes"
		if legacy {
			name = "deprecated blocks"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			planned := securityGroupTestUnknownRuleList(t, true)
			configured := securityGroupTestUnknownRuleList(t, false)
			state, plan, config := securityGroupTestModel(), securityGroupTestModel(), securityGroupTestModel()
			state.InboundRules, state.InboundRule = prior, null
			state.OutboundRules, plan.OutboundRules, config.OutboundRules = empty, empty, empty
			state.OutboundRule, plan.OutboundRule, config.OutboundRule = null, null, null
			if legacy {
				plan.InboundRule, config.InboundRule = planned, configured
				plan.InboundRules, config.InboundRules = null, null
			} else {
				plan.InboundRules, config.InboundRules = planned, configured
				plan.InboundRule, config.InboundRule = null, null
			}

			response := runSecurityGroupModifyPlan(t, state, plan, config)
			require.False(t, response.Diagnostics.HasError(), response.Diagnostics.Errors())
			var got SecurityGroupResourceModel
			diags := response.Plan.Get(context.Background(), &got)
			require.False(t, diags.HasError(), diags.Errors())
			selected := got.InboundRules
			if legacy {
				selected = got.InboundRule
				assert.True(t, got.InboundRules.IsNull())
			} else {
				assert.True(t, got.InboundRule.IsNull())
			}
			rule := selected.Elements()[0].(types.Object).Attributes()
			assert.True(t, rule["ports"].IsUnknown())
			assert.True(t, rule["id"].IsUnknown())
			assert.True(t, rule["direction"].IsUnknown())
		})
	}
}

func TestSecurityGroupRuleListHasUnknownConfiguredFields(t *testing.T) {
	t.Parallel()

	base := SecurityGroupRuleModel{
		ID: types.StringUnknown(), Direction: types.StringUnknown(),
		Protocol: caseInsensitiveStringValueOf("TCP"), Ports: types.StringNull(),
		Targets: types.SetValueMust(types.StringType, []attr.Value{types.StringValue("all:ipv4")}),
		Notes:   types.StringNull(),
	}
	for _, test := range []struct {
		name   string
		mutate func(*SecurityGroupRuleModel)
	}{
		{name: "custom protocol", mutate: func(rule *SecurityGroupRuleModel) {
			rule.Protocol = caseInsensitiveStringValue{StringValue: types.StringUnknown()}
		}},
		{name: "ports", mutate: func(rule *SecurityGroupRuleModel) { rule.Ports = types.StringUnknown() }},
		{name: "notes", mutate: func(rule *SecurityGroupRuleModel) { rule.Notes = types.StringUnknown() }},
		{
			name: "targets collection",
			mutate: func(rule *SecurityGroupRuleModel) {
				rule.Targets = types.SetUnknown(types.StringType)
			},
		},
		{name: "target element", mutate: func(rule *SecurityGroupRuleModel) {
			rule.Targets = types.SetValueMust(types.StringType, []attr.Value{types.StringUnknown()})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			rule := base
			test.mutate(&rule)
			value := securityGroupTestRuleModelList(t, rule)
			assert.True(t, securityGroupRuleListHasUnknownConfiguredFields(value))
		})
	}
	objectUnknown := types.ListValueMust(securityGroupRuleObjectType(), []attr.Value{
		types.ObjectUnknown(securityGroupRuleObjectType().AttrTypes),
	})
	assert.True(t, securityGroupRuleListHasUnknownConfiguredFields(objectUnknown))
	assert.False(t, securityGroupRuleListHasUnknownConfiguredFields(securityGroupTestRuleModelList(t, base)))
}

func TestSecurityGroupModifyPlanExternalAdoptionUsesConfiguredRepresentation(t *testing.T) {
	t.Parallel()

	rule := canonicalSecurityGroupRule{
		Direction: "inbound", Protocol: "TCP", Ports: "2222",
		Targets: []string{"all:ipv4"},
	}
	configured := securityGroupTestRuleList(t, []canonicalSecurityGroupRule{rule}, true)
	null := types.ListNull(securityGroupRuleObjectType())
	empty := types.ListValueMust(securityGroupRuleObjectType(), nil)

	for _, legacy := range []bool{false, true} {
		name := "plural attributes"
		if legacy {
			name = "deprecated blocks"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			state, plan, config := securityGroupTestModel(), securityGroupTestModel(), securityGroupTestModel()
			state.ExternalRules = types.BoolValue(true)
			plan.ExternalRules, config.ExternalRules = types.BoolValue(false), types.BoolNull()
			state.InboundRules, state.InboundRule = null, null
			state.OutboundRules, state.OutboundRule = null, null
			plan.OutboundRules, config.OutboundRules = empty, empty
			plan.OutboundRule, config.OutboundRule = null, null
			if legacy {
				plan.InboundRule, config.InboundRule = configured, configured
				plan.InboundRules, config.InboundRules = null, null
			} else {
				plan.InboundRules, config.InboundRules = configured, configured
				plan.InboundRule, config.InboundRule = null, null
			}

			response := runSecurityGroupModifyPlan(t, state, plan, config)
			require.False(t, response.Diagnostics.HasError(), response.Diagnostics.Errors())
			var got SecurityGroupResourceModel
			diags := response.Plan.Get(context.Background(), &got)
			require.False(t, diags.HasError(), diags.Errors())
			if legacy {
				rules := got.InboundRule.Elements()[0].(types.Object).Attributes()
				assert.True(t, rules["id"].IsUnknown())
				assert.True(t, rules["direction"].IsUnknown())
				assert.Equal(t, "TCP", rules["protocol"].(caseInsensitiveStringValue).ValueString())
				assert.Equal(t, "2222", rules["ports"].(types.String).ValueString())
				assert.True(t, got.InboundRules.IsNull())
			} else {
				rules := got.InboundRules.Elements()[0].(types.Object).Attributes()
				assert.True(t, rules["id"].IsUnknown())
				assert.True(t, rules["direction"].IsUnknown())
				assert.Equal(t, "TCP", rules["protocol"].(caseInsensitiveStringValue).ValueString())
				assert.Equal(t, "2222", rules["ports"].(types.String).ValueString())
				assert.True(t, got.InboundRule.IsNull())
			}
		})
	}
}

func TestSecurityGroupPluralExternalAdoptionModifyPlanFeedsUpdate(t *testing.T) {
	t.Parallel()

	var mutations atomic.Int32
	client := newVirtualMachineTestClient(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			mutations.Add(1)
			http.Error(writer, "unexpected mutation", http.StatusInternalServerError)
			return
		}
		switch request.URL.Path {
		case "/security_groups/security_group":
			writeTestJSON(writer, http.StatusOK, `{
				"security_group": {
					"id": "security_group_test", "name": "Test",
					"allow_all_inbound": false, "allow_all_outbound": false,
					"associations": []
				}
			}`)
		case "/security_groups/security_group/rules":
			writeTestJSON(writer, http.StatusOK, `{
				"pagination": {"total_pages": 1},
				"security_group_rules": [{
					"id": "rule-1", "direction": "inbound", "protocol": "TCP",
					"ports": "22", "targets": ["all:ipv4"], "notes": "SSH"
				}]
			}`)
		default:
			http.NotFound(writer, request)
		}
	})
	resourceUnderTest := &SecurityGroupResource{M: &Meta{Core: client, testMode: true}}
	null := types.ListNull(securityGroupRuleObjectType())
	empty := types.ListValueMust(securityGroupRuleObjectType(), nil)
	desired := securityGroupTestRuleList(t, []canonicalSecurityGroupRule{{
		Direction: "inbound", Protocol: "TCP", Ports: "2222",
		Targets: []string{"all:ipv4"}, Notes: "SSH",
	}}, true)
	state, plan, config := securityGroupTestModel(), securityGroupTestModel(), securityGroupTestModel()
	state.ExternalRules = types.BoolValue(true)
	plan.ExternalRules = types.BoolValue(true)
	config.ExternalRules = types.BoolNull()
	state.InboundRules, state.OutboundRules = null, null
	state.InboundRule, state.OutboundRule = null, null
	plan.InboundRules, config.InboundRules = desired, desired
	plan.OutboundRules, config.OutboundRules = empty, empty
	plan.InboundRule, config.InboundRule = null, null
	plan.OutboundRule, config.OutboundRule = null, null

	stateValue := securityGroupTestState(t, state)
	planValue := securityGroupTestState(t, plan)
	configValue := securityGroupTestState(t, config)
	modifyRequest := resource.ModifyPlanRequest{
		State: stateValue, Plan: tfsdk.Plan(planValue), Config: tfsdk.Config(configValue),
	}
	modifyResponse := resource.ModifyPlanResponse{Plan: tfsdk.Plan(planValue)}
	initializeResourcePrivateState(t, &modifyRequest, &modifyResponse)
	resourceUnderTest.ModifyPlan(context.Background(), modifyRequest, &modifyResponse)
	require.False(t, modifyResponse.Diagnostics.HasError(), modifyResponse.Diagnostics.Errors())

	var modifiedPlan SecurityGroupResourceModel
	diagnostics := modifyResponse.Plan.Get(context.Background(), &modifiedPlan)
	require.False(t, diagnostics.HasError(), diagnostics.Errors())
	require.False(t, modifiedPlan.InboundRules.IsUnknown())
	require.Empty(t, modifiedPlan.OutboundRules.Elements())
	plannedRule := modifiedPlan.InboundRules.Elements()[0].(types.Object).Attributes()
	assert.True(t, plannedRule["id"].IsUnknown())
	assert.True(t, plannedRule["direction"].IsUnknown())
	assert.Equal(t, "2222", plannedRule["ports"].(types.String).ValueString())

	updateRequest := resource.UpdateRequest{
		Config: tfsdk.Config(configValue), State: stateValue,
		Plan: modifyResponse.Plan, Private: modifyResponse.Private,
	}
	updateResponse := resource.UpdateResponse{
		State: tfsdk.State(modifyResponse.Plan), Private: modifyResponse.Private,
	}
	resourceUnderTest.Update(context.Background(), updateRequest, &updateResponse)

	require.False(t, updateResponse.Diagnostics.HasError(), updateResponse.Diagnostics.Errors())
	assert.Zero(t, mutations.Load())
	marker, diagnostics := updateResponse.Private.GetKey(
		context.Background(), securityGroupExternalAdoptionPrivateKey,
	)
	require.False(t, diagnostics.HasError(), diagnostics.Errors())
	assert.NotEmpty(t, marker)
	var got SecurityGroupResourceModel
	diagnostics = updateResponse.State.Get(context.Background(), &got)
	require.False(t, diagnostics.HasError(), diagnostics.Errors())
	rules, err := listRules(context.Background(), got.InboundRules, securityGroupDirectionInbound)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	assert.Equal(t, "rule-1", rules[0].ID)
	assert.Equal(t, "2222", rules[0].Ports)
}

//nolint:lll // Realistic positional IDs make each reorder failure mode explicit.
func TestSecurityGroupModifyPlanRepairsPositionalRuleIDsBySemantics(t *testing.T) {
	t.Parallel()

	ssh := canonicalSecurityGroupRule{ID: "ssh", Direction: "inbound", Protocol: "TCP", Ports: "22", Targets: []string{"all:ipv4"}, Notes: "SSH"}
	dns := canonicalSecurityGroupRule{ID: "dns", Direction: "inbound", Protocol: "UDP", Ports: "53", Targets: []string{"all:ipv4"}, Notes: "DNS"}
	web := canonicalSecurityGroupRule{ID: "web", Direction: "inbound", Protocol: "TCP", Ports: "443", Targets: []string{"all:ipv6"}, Notes: "HTTPS"}
	newRule := canonicalSecurityGroupRule{Direction: "inbound", Protocol: "ICMP", Targets: []string{"all:ipv4"}, Notes: "Ping"}

	for _, test := range []struct {
		name                   string
		prior                  []canonicalSecurityGroupRule
		positional             []canonicalSecurityGroupRule
		configured             []canonicalSecurityGroupRule
		expectedIDs            []string
		create, update, delete int
	}{
		{
			name: "reorder", prior: []canonicalSecurityGroupRule{ssh, dns},
			positional: []canonicalSecurityGroupRule{
				{ID: "ssh", Direction: dns.Direction, Protocol: dns.Protocol, Ports: dns.Ports, Targets: dns.Targets, Notes: dns.Notes},
				{ID: "dns", Direction: ssh.Direction, Protocol: ssh.Protocol, Ports: ssh.Ports, Targets: ssh.Targets, Notes: ssh.Notes},
			},
			configured: []canonicalSecurityGroupRule{dns, ssh}, expectedIDs: []string{"dns", "ssh"},
		},
		{
			name: "reorder with single material edit", prior: []canonicalSecurityGroupRule{ssh, dns},
			positional: []canonicalSecurityGroupRule{
				{ID: "ssh", Direction: dns.Direction, Protocol: dns.Protocol, Ports: dns.Ports, Targets: dns.Targets, Notes: dns.Notes},
				{ID: "dns", Direction: ssh.Direction, Protocol: ssh.Protocol, Ports: "2222", Targets: ssh.Targets, Notes: ssh.Notes},
			},
			configured: []canonicalSecurityGroupRule{
				dns,
				{Direction: ssh.Direction, Protocol: ssh.Protocol, Ports: "2222", Targets: ssh.Targets, Notes: ssh.Notes},
			},
			expectedIDs: []string{"dns", "ssh"},
			update:      1,
		},
		{
			name: "front insert", prior: []canonicalSecurityGroupRule{ssh, dns},
			positional: []canonicalSecurityGroupRule{
				{ID: "ssh", Direction: newRule.Direction, Protocol: newRule.Protocol, Targets: newRule.Targets, Notes: newRule.Notes},
				{ID: "dns", Direction: ssh.Direction, Protocol: ssh.Protocol, Ports: ssh.Ports, Targets: ssh.Targets, Notes: ssh.Notes},
				{Direction: dns.Direction, Protocol: dns.Protocol, Ports: dns.Ports, Targets: dns.Targets, Notes: dns.Notes},
			},
			configured: []canonicalSecurityGroupRule{newRule, ssh, dns}, expectedIDs: []string{"", "ssh", "dns"}, create: 1,
		},
		{
			name: "front removal", prior: []canonicalSecurityGroupRule{ssh, dns, web},
			positional: []canonicalSecurityGroupRule{
				{ID: "ssh", Direction: dns.Direction, Protocol: dns.Protocol, Ports: dns.Ports, Targets: dns.Targets, Notes: dns.Notes},
				{ID: "dns", Direction: web.Direction, Protocol: web.Protocol, Ports: web.Ports, Targets: web.Targets, Notes: web.Notes},
			},
			configured: []canonicalSecurityGroupRule{dns, web}, expectedIDs: []string{"dns", "web"}, delete: 1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			state, plan, config := securityGroupTestModel(), securityGroupTestModel(), securityGroupTestModel()
			empty := types.ListValueMust(securityGroupRuleObjectType(), nil)
			null := types.ListNull(securityGroupRuleObjectType())
			state.InboundRules = securityGroupTestRuleList(t, test.prior, false)
			plan.InboundRules = securityGroupTestRuleList(t, test.positional, false)
			config.InboundRules = securityGroupTestRuleList(t, test.configured, true)
			state.InboundRule, plan.InboundRule, config.InboundRule = null, null, null
			state.OutboundRules, plan.OutboundRules, config.OutboundRules = empty, empty, empty
			state.OutboundRule, plan.OutboundRule, config.OutboundRule = null, null, null

			response := runSecurityGroupModifyPlan(t, state, plan, config)
			require.False(t, response.Diagnostics.HasError(), response.Diagnostics.Errors())
			var got SecurityGroupResourceModel
			diags := response.Plan.Get(context.Background(), &got)
			require.False(t, diags.HasError(), diags.Errors())
			desired, err := listRules(context.Background(), got.InboundRules, securityGroupDirectionInbound)
			require.NoError(t, err)
			ids := make([]string, len(desired))
			for i := range desired {
				ids[i] = desired[i].ID
			}
			assert.Equal(t, test.expectedIDs, ids)
			reconciliation := classifySecurityGroupRuleReconciliation(test.prior, desired)
			assert.Len(t, reconciliation.CreateIndexes, test.create)
			assert.Len(t, reconciliation.UpdateIndexes, test.update)
			assert.Len(t, reconciliation.Delete, test.delete)
		})
	}
}

func TestSecurityGroupRuleCanonicalizationNormalizesOptionalValues(t *testing.T) {
	t.Parallel()

	targets := types.SetValueMust(types.StringType, []attr.Value{
		types.StringValue("all:ipv6"), types.StringValue("all:ipv4"),
	})
	model := SecurityGroupRuleModel{
		ID: types.StringNull(), Direction: types.StringNull(),
		Protocol: caseInsensitiveStringValueOf("tcp"), Ports: types.StringNull(),
		Targets: targets, Notes: types.StringNull(),
	}
	value, diags := types.ListValueFrom(
		context.Background(), securityGroupRuleObjectType(), []SecurityGroupRuleModel{model},
	)
	require.False(t, diags.HasError(), diags.Errors())
	rules, diags := canonicalRulesFromList(context.Background(), value, securityGroupDirectionInbound)
	require.False(t, diags.HasError(), diags.Errors())
	require.Len(t, rules, 1)
	assert.Equal(t, securityGroupDirectionInbound, rules[0].Direction)
	assert.Equal(t, "tcp", rules[0].Protocol)
	assert.Empty(t, rules[0].Ports)
	assert.Empty(t, rules[0].Notes)
	assert.Equal(t, canonicalSecurityGroupRule{
		Direction: "INBOUND", Protocol: "TCP", Targets: []string{"all:ipv4", "all:ipv6"},
	}.fingerprint(), rules[0].fingerprint())

	arguments := securityGroupRuleAPIArguments(rules[0])
	require.NotNil(t, arguments.Direction)
	require.NotNil(t, arguments.Protocol)
	require.NotNil(t, arguments.Ports)
	require.NotNil(t, arguments.Notes)
	assert.Equal(t, "inbound", string(*arguments.Direction))
	assert.Equal(t, "TCP", string(*arguments.Protocol))
	assert.Empty(t, *arguments.Ports)
	assert.Empty(t, *arguments.Notes)
}

func TestPreserveCaseInsensitiveCasing(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "INBOUND", preserveCaseInsensitiveCasing("INBOUND", "inbound"))
	assert.Equal(t, "inbound", preserveCaseInsensitiveCasing("", "inbound"))
	assert.Equal(t, "outbound", preserveCaseInsensitiveCasing("INBOUND", "outbound"))
}

func TestSecurityGroupRulePropertiesEqualIgnoresSemanticCasing(t *testing.T) {
	t.Parallel()

	state := SecurityGroupRuleResourceModel{
		SecurityGroupID: types.StringValue("group-1"), Direction: types.StringValue("inbound"),
		Protocol: caseInsensitiveStringValueOf("tcp"), Ports: types.StringValue("22"),
		Targets: types.SetValueMust(types.StringType, []attr.Value{types.StringValue("all:ipv4")}),
		Notes:   types.StringValue("SSH"),
	}
	plan := state
	plan.Direction = types.StringValue("INBOUND")
	plan.Protocol = caseInsensitiveStringValueOf("TCP")
	assert.True(t, securityGroupRulePropertiesEqual(state, plan))

	plan.Ports = types.StringValue("2222")
	assert.False(t, securityGroupRulePropertiesEqual(state, plan))
}

func TestSecurityGroupPaginationTotalPages(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		configure func(*core.PaginationObject)
		want      int
		wantError string
	}{
		{name: "unspecified", wantError: "value is not specified"},
		{
			name: "null",
			configure: func(pagination *core.PaginationObject) {
				pagination.TotalPages.SetNull()
			},
			wantError: "value is null",
		},
		{
			name: "known",
			configure: func(pagination *core.PaginationObject) {
				pagination.TotalPages.Set(3)
			},
			want: 3,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var pagination core.PaginationObject
			if test.configure != nil {
				test.configure(&pagination)
			}
			got, err := securityGroupPaginationTotalPages(pagination, "security groups")
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				require.ErrorContains(t, err, "security groups pagination total_pages")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestSecurityGroupModifyPlanConsumesExternalAdoptionMarkerAfterReconciliation(t *testing.T) {
	t.Parallel()

	rule := canonicalSecurityGroupRule{
		ID: "rule-1", Direction: "inbound", Protocol: "TCP", Ports: "22",
		Targets: []string{"all:ipv4"}, Notes: "SSH",
	}
	for _, test := range []struct {
		name       string
		prior      []canonicalSecurityGroupRule
		wantMarker bool
	}{
		{
			name:  "adopted rules require second reconciliation",
			prior: []canonicalSecurityGroupRule{rule}, wantMarker: true,
		},
		{name: "already empty adoption clears marker"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			state, plan, config := securityGroupTestModel(), securityGroupTestModel(), securityGroupTestModel()
			state.InboundRules = securityGroupTestRuleList(t, test.prior, false)
			state.OutboundRules = securityGroupTestRuleList(t, nil, false)
			plan.InboundRules, plan.OutboundRules = state.InboundRules, state.OutboundRules
			null := types.ListNull(securityGroupRuleObjectType())
			config.InboundRules, config.OutboundRules = null, null
			state.InboundRule, state.OutboundRule = null, null
			plan.InboundRule, plan.OutboundRule = null, null
			config.InboundRule, config.OutboundRule = null, null

			stateValue := securityGroupTestState(t, state)
			planValue := securityGroupTestState(t, plan)
			configValue := securityGroupTestState(t, config)
			request := resource.ModifyPlanRequest{
				State: stateValue, Plan: tfsdk.Plan(planValue), Config: tfsdk.Config(configValue),
			}
			response := resource.ModifyPlanResponse{Plan: tfsdk.Plan(planValue)}
			initializeResourcePrivateState(t, &request, &response)
			require.False(t, request.Private.SetKey(
				context.Background(), securityGroupExternalAdoptionPrivateKey, []byte("true"),
			).HasError())

			(&SecurityGroupResource{}).ModifyPlan(context.Background(), request, &response)

			require.False(t, response.Diagnostics.HasError(), response.Diagnostics.Errors())
			var got SecurityGroupResourceModel
			diags := response.Plan.Get(context.Background(), &got)
			require.False(t, diags.HasError(), diags.Errors())
			assert.Empty(t, got.InboundRules.Elements())
			marker, diags := response.Private.GetKey(context.Background(), securityGroupExternalAdoptionPrivateKey)
			require.False(t, diags.HasError(), diags.Errors())
			assert.Equal(t, test.wantMarker, len(marker) > 0)
		})
	}
}

func TestSecurityGroupUpdateDoesNotPersistEmptyExternalAdoptionMarker(t *testing.T) {
	t.Parallel()

	client := newVirtualMachineTestClient(t, func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/security_groups/security_group":
			writeTestJSON(writer, http.StatusOK, `{
				"security_group": {
					"id": "security_group_test", "name": "Test",
					"allow_all_inbound": false, "allow_all_outbound": false,
					"associations": []
				}
			}`)
		case "/security_groups/security_group/rules":
			writeTestJSON(writer, http.StatusOK, `{
				"pagination": {"total_pages": 1}, "security_group_rules": []
			}`)
		default:
			http.NotFound(writer, request)
		}
	})
	r := &SecurityGroupResource{M: &Meta{Core: client, testMode: true}}
	state, plan, config := securityGroupTestModel(), securityGroupTestModel(), securityGroupTestModel()
	state.ExternalRules = types.BoolValue(true)
	plan.ExternalRules = types.BoolValue(false)
	unknown := types.ListUnknown(securityGroupRuleObjectType())
	plan.InboundRules, plan.OutboundRules = unknown, unknown
	stateValue := securityGroupTestState(t, state)
	planValue := securityGroupTestState(t, plan)
	configValue := securityGroupTestState(t, config)
	request := resource.UpdateRequest{
		Config: tfsdk.Config(configValue), State: stateValue, Plan: tfsdk.Plan(planValue),
	}
	response := resource.UpdateResponse{State: planValue}
	initializeResourcePrivateState(t, &request, &response)

	r.Update(context.Background(), request, &response)

	require.False(t, response.Diagnostics.HasError(), response.Diagnostics.Errors())
	marker, diags := response.Private.GetKey(context.Background(), securityGroupExternalAdoptionPrivateKey)
	require.False(t, diags.HasError(), diags.Errors())
	assert.Empty(t, marker)
}

func TestSecurityGroupReadConsumesImportMarker(t *testing.T) {
	t.Parallel()

	client := newVirtualMachineTestClient(t, func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/security_groups/security_group":
			writeTestJSON(writer, http.StatusOK, `{
				"security_group": {
					"id": "security_group_test", "name": "Test",
					"allow_all_inbound": false, "allow_all_outbound": false,
					"associations": []
				}
			}`)
		case "/security_groups/security_group/rules":
			writeTestJSON(writer, http.StatusOK, `{
				"pagination": {"total_pages": 1}, "security_group_rules": []
			}`)
		default:
			http.NotFound(writer, request)
		}
	})
	r := &SecurityGroupResource{M: &Meta{Core: client, testMode: true}}
	state := securityGroupTestState(t, securityGroupTestModel())
	request := resource.ReadRequest{State: state}
	response := resource.ReadResponse{State: state}
	initializeResourcePrivateState(t, &request, &response)
	require.False(t, request.Private.SetKey(
		context.Background(), securityGroupImportPrivateKey, []byte("true"),
	).HasError())

	r.Read(context.Background(), request, &response)

	require.False(t, response.Diagnostics.HasError(), response.Diagnostics.Errors())
	marker, diags := response.Private.GetKey(context.Background(), securityGroupImportPrivateKey)
	require.False(t, diags.HasError(), diags.Errors())
	assert.Empty(t, marker)
}

func securityGroupTestRuleList(
	t *testing.T,
	rules []canonicalSecurityGroupRule,
	plan bool,
) types.List {
	t.Helper()
	var value types.List
	var diags diag.Diagnostics
	if plan {
		value, diags = securityGroupRulesPlanListValue(context.Background(), rules)
	} else {
		value, diags = securityGroupRulesListValue(context.Background(), rules)
	}
	require.False(t, diags.HasError(), diags.Errors())
	return value
}

func securityGroupTestRuleModelList(t *testing.T, rule SecurityGroupRuleModel) types.List {
	t.Helper()
	value, diags := types.ListValueFrom(
		context.Background(), securityGroupRuleObjectType(), []SecurityGroupRuleModel{rule},
	)
	require.False(t, diags.HasError(), diags.Errors())
	return value
}

func securityGroupTestUnknownRuleList(t *testing.T, planned bool) types.List {
	t.Helper()
	id, direction := types.StringUnknown(), types.StringUnknown()
	if planned {
		id = types.StringValue("rule-1")
		direction = types.StringValue("inbound")
	}
	return securityGroupTestRuleModelList(t, SecurityGroupRuleModel{
		ID: id, Direction: direction,
		Protocol: caseInsensitiveStringValueOf("TCP"), Ports: types.StringUnknown(),
		Targets: types.SetValueMust(types.StringType, []attr.Value{types.StringValue("all:ipv4")}),
		Notes:   types.StringValue("SSH"),
	})
}

func securityGroupTestModel() SecurityGroupResourceModel {
	null := types.ListNull(securityGroupRuleObjectType())
	return SecurityGroupResourceModel{
		ID: types.StringValue("security_group_test"), Name: types.StringValue("Test"),
		Associations:    types.SetValueMust(types.StringType, nil),
		AllowAllInbound: types.BoolValue(false), AllowAllOutbound: types.BoolValue(false),
		ExternalRules: types.BoolValue(false),
		InboundRules:  null, OutboundRules: null, InboundRule: null, OutboundRule: null,
	}
}

func securityGroupTestState(t *testing.T, model SecurityGroupResourceModel) tfsdk.State {
	t.Helper()
	r := &SecurityGroupResource{}
	var schemaResponse resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schemaResponse)
	require.False(t, schemaResponse.Diagnostics.HasError(), schemaResponse.Diagnostics.Errors())
	state := tfsdk.State{Schema: schemaResponse.Schema}
	diags := state.Set(context.Background(), model)
	require.False(t, diags.HasError(), diags.Errors())
	return state
}

func securityGroupRuleTestState(t *testing.T, model SecurityGroupRuleResourceModel) tfsdk.State {
	t.Helper()
	var schemaResponse resource.SchemaResponse
	(&SecurityGroupRuleResource{}).Schema(context.Background(), resource.SchemaRequest{}, &schemaResponse)
	require.False(t, schemaResponse.Diagnostics.HasError(), schemaResponse.Diagnostics.Errors())
	state := tfsdk.State{Schema: schemaResponse.Schema}
	diagnostics := state.Set(context.Background(), model)
	require.False(t, diagnostics.HasError(), diagnostics.Errors())
	return state
}

func runSecurityGroupSetValidators(attribute schema.SetAttribute, value types.Set) diag.Diagnostics {
	var diagnostics diag.Diagnostics
	for _, setValidator := range attribute.Validators {
		response := schemavalidator.SetResponse{}
		setValidator.ValidateSet(context.Background(), schemavalidator.SetRequest{
			Path: path.Root("test"), ConfigValue: value,
		}, &response)
		diagnostics.Append(response.Diagnostics...)
	}
	return diagnostics
}

func runSecurityGroupValidateConfig(
	t *testing.T,
	model SecurityGroupResourceModel,
) resource.ValidateConfigResponse {
	t.Helper()
	state := securityGroupTestState(t, model)
	response := resource.ValidateConfigResponse{}
	(&SecurityGroupResource{}).ValidateConfig(context.Background(), resource.ValidateConfigRequest{
		Config: tfsdk.Config(state),
	}, &response)
	return response
}

func runSecurityGroupModifyPlan(
	t *testing.T,
	stateModel, planModel, configModel SecurityGroupResourceModel,
) resource.ModifyPlanResponse {
	t.Helper()
	state := securityGroupTestState(t, stateModel)
	plan := securityGroupTestState(t, planModel)
	config := securityGroupTestState(t, configModel)
	response := resource.ModifyPlanResponse{Plan: tfsdk.Plan(plan)}
	(&SecurityGroupResource{}).ModifyPlan(context.Background(), resource.ModifyPlanRequest{
		State: state, Plan: tfsdk.Plan(plan),
		Config: tfsdk.Config(config),
	}, &response)
	return response
}
