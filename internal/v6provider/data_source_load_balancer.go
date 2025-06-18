package v6provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	core "github.com/krystal/go-katapult/next/core"
)

type (
	LoadBalancerDataSource struct {
		M *Meta
	}

	LoadBalancerDataSourceModel struct {
		ID                     types.String `tfsdk:"id"`
		Name                   types.String `tfsdk:"name"`
		VirtualMachineIDs      types.Set    `tfsdk:"virtual_machine_ids"`
		VirtualMachineGroupIDs types.Set    `tfsdk:"virtual_machine_group_ids"`
		TagIDs                 types.Set    `tfsdk:"tag_ids"`
		IPAddress              types.String `tfsdk:"ip_address"`
		HTTPSRedirect          types.Bool   `tfsdk:"https_redirect"`
	}
)

func (ds *LoadBalancerDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_load_balancer"
}

func (ds *LoadBalancerDataSource) Configure(
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

func loadBalancerDataSourceSchemaAttrs() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Required: true,
		},
		"name": schema.StringAttribute{
			Computed: true,
		},
		"virtual_machine_ids": schema.SetAttribute{
			Computed:    true,
			ElementType: types.StringType,
		},
		"virtual_machine_group_ids": schema.SetAttribute{
			Computed:    true,
			ElementType: types.StringType,
		},
		"tag_ids": schema.SetAttribute{
			Computed:    true,
			ElementType: types.StringType,
		},
		"ip_address": schema.StringAttribute{
			Computed: true,
		},
		"https_redirect": schema.BoolAttribute{
			Computed: true,
		},
	}
}

func (ds *LoadBalancerDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Attributes: loadBalancerDataSourceSchemaAttrs(),
	}
}

func (ds *LoadBalancerDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var data LoadBalancerDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	res, err := ds.M.Core.GetLoadBalancerWithResponse(
		ctx,
		&core.GetLoadBalancerParams{
			LoadBalancerId: data.ID.ValueStringPointer(),
		})
	if err != nil {
		if res != nil {
			err = genericAPIError(err, res.Body)
		}

		resp.Diagnostics.AddError("Load Balancer Error", err.Error())
		return
	}
	lb := res.JSON200.LoadBalancer
	data.Name = types.StringPointerValue(lb.Name)
	data.HTTPSRedirect = types.BoolPointerValue(lb.HttpsRedirect)
	if lb.IpAddress != nil {
		data.IPAddress = types.StringPointerValue(lb.IpAddress.Address)
	}

	data.VirtualMachineIDs = types.SetNull(types.StringType)
	data.TagIDs = types.SetNull(types.StringType)
	data.VirtualMachineGroupIDs = types.SetNull(types.StringType)
	if lb.ResourceIds != nil {
		list := flattenLoadBalancerResourceIDs(*lb.ResourceIds)

		switch *lb.ResourceType {
		case core.VirtualMachines:
			data.VirtualMachineIDs = list
		case core.VirtualMachineGroups:
			data.VirtualMachineGroupIDs = list
		case core.Tags:
			data.TagIDs = list
		}
	}

	data.ID = types.StringPointerValue(lb.Id)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
