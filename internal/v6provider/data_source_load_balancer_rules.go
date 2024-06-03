package v6provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/core"
)

type (
	LoadBalancerRulesDataSource struct {
		M *Meta
	}
	LoadBalancerRulesDataSourceModel struct {
		ID    types.String `tfsdk:"id"`
		Rules types.List   `tfsdk:"rules"`
	}
)

func (ds *LoadBalancerRulesDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_load_balancer_rules"
}

func (ds *LoadBalancerRulesDataSource) Configure(
	_ context.Context,
	req datasource.ConfigureRequest,
	resp *datasource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		return
	}

	m, ok := req.ProviderData.(*Meta)
	if !ok {
		resp.Diagnostics.AddError(
			"Meta Error",
			"meta is not of type *Meta",
		)
		return
	}

	ds.M = m
}

func (ds *LoadBalancerRulesDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:    true,
				Description: "The unique identifier for the Load Balancer.",
			},
			"rules": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: loadBalancerRuleDataSourceSchemaAttrs(),
				},
			},
		},
	}
}

func (ds *LoadBalancerRulesDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var data LoadBalancerRulesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	rules, err := getLBRules(ctx, ds.M,
		core.LoadBalancerRef{
			ID: data.ID.ValueString(),
		})
	if err != nil {
		resp.Diagnostics.AddError("Load Balancer Rules Error", err.Error())

		return
	}

	fmt.Println(rules)

	data.Rules = types.ListValueMust(
		LoadBalancerRuleType(),
		convertCoreLBRulesToAttrValue(rules),
	)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func getLBRules(
	ctx context.Context,
	m *Meta,
	lbRef core.LoadBalancerRef,
) ([]*core.LoadBalancerRule, error) {
	var rules []*core.LoadBalancerRule

	totalPages := 2
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		pageResult, resp, err := m.Core.LoadBalancerRules.List(
			ctx, lbRef, &core.ListOptions{Page: pageNum},
		)
		if err != nil {
			return nil, err
		}

		totalPages = resp.Pagination.TotalPages
		rules = append(rules, pageResult...)
	}

	for i, rl := range rules {
		rule, _, err := m.Core.LoadBalancerRules.GetByID(
			ctx, rl.ID,
		)
		if err != nil {
			return nil, err
		}

		rules[i] = rule
	}

	return rules, nil
}

func convertCoreLBRulesToAttrValue(
	rules []*core.LoadBalancerRule,
) []attr.Value {
	attrs := make([]attr.Value, len(rules))
	for i, r := range rules {
		attrs[i] = types.ObjectValueMust(
			LoadBalancerRuleType().AttrTypes,
			map[string]attr.Value{
				"id":               types.StringValue(r.ID),
				"load_balancer_id": types.StringNull(),
				"algorithm":        types.StringValue(string(r.Algorithm)),
				"protocol":         types.StringValue(string(r.Protocol)),
				"listen_port":      types.Int64Value(int64(r.ListenPort)),
				"destination_port": types.Int64Value(int64(r.DestinationPort)),
				"proxy_protocol":   types.BoolValue(r.ProxyProtocol),
				"backend_ssl":      types.BoolValue(r.BackendSSL),
				"passthrough_ssl":  types.BoolValue(r.PassthroughSSL),
				"certificate_ids": types.SetValueMust(
					CertificateType(),
					ConvertCoreCertsToTFValues(r.Certificates),
				),
				"check_enabled":  types.BoolValue(r.CheckEnabled),
				"check_fall":     types.Int64Value(int64(r.CheckFall)),
				"check_interval": types.Int64Value(int64(r.CheckInterval)),
				"check_path":     types.StringValue(r.CheckPath),
				"check_protocol": types.StringValue(string(r.CheckProtocol)),
				"check_rise":     types.Int64Value(int64(r.CheckRise)),
				"check_timeout":  types.Int64Value(int64(r.CheckTimeout)),
				"check_http_statuses": types.StringValue(
					string(r.CheckHTTPStatuses),
				),
			},
		)
	}

	return attrs
}
