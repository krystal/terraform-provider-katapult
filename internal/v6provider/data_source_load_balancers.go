package v6provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
)

type (
	LoadBalancersDataSource struct {
		M *Meta
	}

	LoadBalancersDataSourceModel struct {
		ID            types.String `tfsdk:"id"`
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

	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Always set to provider organization value.",
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

	loadBalancers := []core.GetLoadBalancer200ResponseLoadBalancer{}
	totalPages := 2
	for pageNum := 1; pageNum < totalPages; pageNum++ {
		res, err := ds.M.Core.GetOrganizationLoadBalancersWithResponse(ctx,
			&core.GetOrganizationLoadBalancersParams{
				OrganizationSubDomain: &ds.M.confOrganization,
				Page:                  &pageNum,
			})
		if err != nil {
			resp.Diagnostics.AddError("Load Balancer List Error", err.Error())
			return
		}

		var totalPagesError error
		totalPages, totalPagesError = res.JSON200.Pagination.TotalPages.Get()
		if totalPagesError != nil {
			resp.Diagnostics.AddError(
				"Load Balancer List Error",
				totalPagesError.Error())
			return
		}

		lbs := res.JSON200.LoadBalancers

		for _, lb := range lbs {
			res, err := ds.M.Core.GetLoadBalancerWithResponse(ctx,
				&core.GetLoadBalancerParams{LoadBalancerId: lb.Id})
			if err != nil {
				resp.Diagnostics.AddError(
					"Load Balancer Get Error",
					err.Error())
				return
			}

			loadBalancers = append(loadBalancers, res.JSON200.LoadBalancer)
		}
	}

	list := make([]attr.Value, len(loadBalancers))

	for i, lb := range loadBalancers {
		attrs := map[string]attr.Value{
			"id":   types.StringPointerValue(lb.Id),
			"name": types.StringPointerValue(lb.Name),
			"https_redirect": types.BoolPointerValue(
				lb.HttpsRedirect,
			),
			"virtual_machine_ids":       types.SetNull(types.StringType),
			"virtual_machine_group_ids": types.SetNull(types.StringType),
			"tag_ids":                   types.SetNull(types.StringType),
		}

		resourceIDs := flattenLoadBalancerResourceIDs(*lb.ResourceIds)
		switch *lb.ResourceType {
		case core.VirtualMachines:
			attrs["virtual_machine_ids"] = resourceIDs
		case core.VirtualMachineGroups:
			attrs["virtual_machine_group_ids"] = resourceIDs
		case core.Tags:
			attrs["tag_ids"] = resourceIDs
		}

		if lb.IpAddress != nil {
			attrs["ip_address"] = types.StringPointerValue(lb.IpAddress.Address)
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
