package v6provider

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
)

type (
	LoadBalancerRulesDataSource struct {
		M *Meta
	}
	LoadBalancerRulesDataSourceModel struct {
		LoadBalancerID types.String `tfsdk:"load_balancer_id"`
		Rules          types.List   `tfsdk:"rules"`
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
			"load_balancer_id": schema.StringAttribute{
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
		data.LoadBalancerID.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Load Balancer Rules Error", err.Error())

		return
	}

	data.Rules = types.ListValueMust(
		LoadBalancerRuleType(),
		convertCoreLBRulesToAttrValue(rules),
	)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func getLBRules(
	ctx context.Context,
	m *Meta,
	lbID string,
) (
	[]core.GetLoadBalancersRulesLoadBalancerRule200ResponseLoadBalancerRule,
	error,
) {
	var ruleList []core.GetLoadBalancerRules200ResponseLoadBalancerRules

	totalPages := 2
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		res, err := m.Core.GetLoadBalancerRulesWithResponse(ctx,
			&core.GetLoadBalancerRulesParams{
				LoadBalancerId: &lbID,
				Page:           &pageNum,
			},
		)
		if err != nil {
			return nil, err
		}

		if res.JSON200 == nil {
			return nil, errors.New("response body is nil")
		}

		var totalPagesError error
		totalPages, totalPagesError = res.JSON200.Pagination.TotalPages.Get()
		if totalPagesError != nil {
			return nil, totalPagesError
		}

		ruleList = append(ruleList, res.JSON200.LoadBalancerRules...)
	}

	rules := make(
		[]core.GetLoadBalancersRulesLoadBalancerRule200ResponseLoadBalancerRule,
		len(ruleList))

	for i, rl := range ruleList {
		res, err := m.Core.
			GetLoadBalancersRulesLoadBalancerRuleWithResponse(ctx,
				&core.GetLoadBalancersRulesLoadBalancerRuleParams{
					LoadBalancerRuleId: rl.Id,
				})
		if err != nil {
			return nil, err
		}

		if res.JSON200 == nil {
			return nil, errors.New("response body is nil")
		}

		rules[i] = res.JSON200.LoadBalancerRule
	}

	return rules, nil
}

func convertCoreLBRulesToAttrValue(
	//nolint:lll // generated type name
	rules []core.GetLoadBalancersRulesLoadBalancerRule200ResponseLoadBalancerRule,
) []attr.Value {
	attrs := make([]attr.Value, len(rules))
	for i, r := range rules {
		checkProtocol, _ := r.CheckProtocol.Get()
		checkHTTPStatuses, _ := r.CheckHttpStatuses.Get()

		attrs[i] = types.ObjectValueMust(
			LoadBalancerRuleType().AttrTypes,
			map[string]attr.Value{
				"id":               types.StringPointerValue(r.Id),
				"load_balancer_id": types.StringNull(),
				"algorithm":        types.StringValue(string(*r.Algorithm)),
				"protocol":         types.StringValue(string(*r.Protocol)),
				"listen_port":      types.Int64Value(int64(*r.ListenPort)),
				"destination_port": types.Int64Value(int64(*r.DestinationPort)),
				"proxy_protocol":   types.BoolPointerValue(r.ProxyProtocol),
				"backend_ssl":      types.BoolPointerValue(r.BackendSsl),
				"passthrough_ssl":  types.BoolPointerValue(r.PassthroughSsl),
				"certificate_ids": types.SetValueMust(
					types.StringType,
					ConvertCoreCertsToTFValues(*r.Certificates),
				),
				"check_enabled":  types.BoolPointerValue(r.CheckEnabled),
				"check_fall":     types.Int64Value(int64(*r.CheckFall)),
				"check_interval": types.Int64Value(int64(*r.CheckInterval)),
				"check_path":     types.StringValue(*r.CheckPath),
				"check_protocol": types.StringValue(string(checkProtocol)),
				"check_rise":     types.Int64Value(int64(*r.CheckRise)),
				"check_timeout":  types.Int64Value(int64(*r.CheckTimeout)),
				"check_http_statuses": types.StringValue(
					string(checkHTTPStatuses),
				),
			},
		)
	}

	return attrs
}
