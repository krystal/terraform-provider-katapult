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
		ID                  types.String `tfsdk:"id"`
		Name                types.String `tfsdk:"name"`
		ResourceType        types.String `tfsdk:"resource_type"`
		VirtualMachine      types.List   `tfsdk:"virtual_machine"`
		VirtualMachineGroup types.List   `tfsdk:"virtual_machine_group"`
		Tag                 types.List   `tfsdk:"tag"`
		IPAddress           types.String `tfsdk:"ip_address"`
		HTTPSRedirect       types.Bool   `tfsdk:"https_redirect"`
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

func (ds *LoadBalancerDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required: true,
			},
			"name": schema.StringAttribute{
				Computed: true,
			},
			"resource_type": schema.StringAttribute{
				Computed: true,
			},
			"virtual_machine": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed: true,
						},
					},
				},
			},
			"virtual_machine_group": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed: true,
						},
					},
				},
			},
			"tag": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed: true,
						},
					},
				},
			},
			"ip_address": schema.StringAttribute{
				Computed: true,
			},
			"https_redirect": schema.BoolAttribute{
				Computed: true,
			},
		},
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
		resp.Diagnostics.AddError("Load Balancer GetByID Error", err.Error())
		return
	}

	lb := res.JSON200.LoadBalancer

	data.Name = types.StringPointerValue(lb.Name)
	if lb.ResourceType != nil {
		data.ResourceType = types.StringValue(string(*lb.ResourceType))
	}

	data.HTTPSRedirect = types.BoolPointerValue(lb.HttpsRedirect)
	if lb.IpAddress != nil {
		data.IPAddress = types.StringPointerValue(lb.IpAddress.Address)
	}

	if lb.ResourceIds != nil {
		list := flattenLoadBalancerResourceIDs(*lb.ResourceIds)
		switch *lb.ResourceType {
		case core.VirtualMachines:
			data.VirtualMachine = list
		case core.VirtualMachineGroups:
			data.VirtualMachineGroup = list
		case core.Tags:
			data.Tag = list
		}
	}

	data.ID = types.StringPointerValue(lb.Id)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
