package v6provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/core"
)

type (
	LoadBalancersDataSource struct {
		M *Meta
	}

	LoadBalancersDataSourceModel struct {
		ID            types.String `tfsdk:"id"`
		IncludeRules  types.Bool   `tfsdk:"include_rules"`
		LoadBalancers types.List   `tfsdk:"load_balancers"`
	}
)

func (ds *LoadBalancersDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_load_balancers"
}

func (ds *LoadBalancersDataSource) Configure(
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

func (ds *LoadBalancersDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	lbAttrs := loadBalancerDataSourceSchemaAttrs()
	delete(lbAttrs, "include_rules")

	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Always set to provider organization value.",
			},
			"include_rules": schema.BoolAttribute{
				Description: "Whether to include rules in the output. Can " +
					"be slow if there are a lot of load balancers, as each " +
					"load balancer requires a separate API " +
					"call to fetch rules.",
				Optional: true,
				Computed: true,
			},
			"load_balancers": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: lbAttrs,
				},
			},
		},
	}
}

func (ds *LoadBalancersDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	data := LoadBalancersDataSourceModel{}
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	loadBalancers := []*core.LoadBalancer{}
	totalPages := 2
	for pageNum := 1; pageNum < totalPages; pageNum++ {
		lbs, res, err := ds.M.Core.LoadBalancers.List(ctx,
			ds.M.OrganizationRef,
			&core.ListOptions{Page: pageNum})
		if err != nil {
			resp.Diagnostics.AddError("Load Balancer List Error", err.Error())
			return
		}

		totalPages = res.Pagination.TotalPages

		for _, lb := range lbs {
			fullLB, _, err := ds.M.Core.LoadBalancers.Get(ctx, lb.Ref())
			if err != nil {
				resp.Diagnostics.AddError(
					"Load Balancer Get Error",
					err.Error())
				return
			}

			loadBalancers = append(loadBalancers, fullLB)
		}
	}

	list := make([]attr.Value, len(loadBalancers))
	resourceIDType := types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"id": types.StringType,
		},
	}

	for i, lb := range loadBalancers {
		attrs := map[string]attr.Value{
			"id":                     types.StringValue(lb.ID),
			"name":                   types.StringValue(lb.Name),
			"resource_type":          types.StringValue(string(lb.ResourceType)), //nolint:lll
			"https_redirect":         types.BoolValue(lb.HTTPSRedirect),
			"virtual_machines":       types.ListNull(resourceIDType),
			"virtual_machine_groups": types.ListNull(resourceIDType),
			"tags":                   types.ListNull(resourceIDType),
		}

		resourceIDs := flattenLoadBalancerResourceIDs(lb.ResourceIDs)
		switch lb.ResourceType {
		case core.VirtualMachinesResourceType:
			attrs["virtual_machines"] = resourceIDs
		case core.VirtualMachineGroupsResourceType:
			attrs["virtual_machine_groups"] = resourceIDs
		case core.TagsResourceType:
			attrs["tags"] = resourceIDs
		}

		if lb.IPAddress != nil {
			attrs["ip_address"] = types.StringValue(lb.IPAddress.Address)
		}

		if data.IncludeRules.ValueBool() {
			rules, err := getLBRules(ctx, ds.M, lb.Ref())
			if err != nil {
				resp.Diagnostics.AddError(
					"Load Balancer Rules Error",
					err.Error())
				return
			}

			attrs["rules"] = types.ListValueMust(
				LoadBalancerRuleType(),
				convertCoreLBRulesToAttrValue(rules),
			)
		} else {
			attrs["rules"] = types.ListNull(LoadBalancerRuleType())
		}

		list[i] = types.ObjectValueMust(
			LoadBalancerType().AttrTypes,
			attrs,
		)
	}

	data.LoadBalancers = types.ListValueMust(
		LoadBalancerType(),
		list,
	)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
