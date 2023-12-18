package v6provider

import (
	"context"

	"github.com/davecgh/go-spew/spew"
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

	data.Rules = types.ListValueMust(
		LoadBalancerRuleType(),
		convertCoreLBRulesToAttrValue(rules),
	)

	spew.Dump(data)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
