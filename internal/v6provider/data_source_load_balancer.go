package v6provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/core"
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

	lb, _, err := ds.M.Core.LoadBalancers.GetByID(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Load Balancer GetByID Error", err.Error())
		return
	}

	data.Name = types.StringValue(lb.Name)
	data.ResourceType = types.StringValue(string(lb.ResourceType))
	data.HTTPSRedirect = types.BoolValue(lb.HTTPSRedirect)
	if lb.IPAddress != nil {
		data.IPAddress = types.StringValue(lb.IPAddress.Address)
	}

	list := flattenLoadBalancerResourceIDs(lb.ResourceIDs)

	switch lb.ResourceType {
	case core.VirtualMachinesResourceType:
		data.VirtualMachine = list
	case core.VirtualMachineGroupsResourceType:
		data.VirtualMachineGroup = list
	case core.TagsResourceType:
		data.Tag = list
	}
	data.ID = types.StringValue(lb.ID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
