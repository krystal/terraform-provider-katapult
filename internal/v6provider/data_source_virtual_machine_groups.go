package v6provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
)

type (
	VirtualMachineGroupsDataSource struct {
		M *Meta
	}

	VirtualMachineGroupsDataSourceModel struct {
		ID     types.String                         `tfsdk:"id"`
		Groups []VirtualMachineGroupDataSourceModel `tfsdk:"groups"`
	}
)

func (d *VirtualMachineGroupsDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_virtual_machine_groups"
}

func (d *VirtualMachineGroupsDataSource) Configure(
	_ context.Context,
	req datasource.ConfigureRequest,
	resp *datasource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		return
	}

	meta, ok := req.ProviderData.(*Meta)
	if !ok {
		resp.Diagnostics.AddError(
			"Meta Error",
			"meta is not of type *Meta",
		)
		return
	}

	d.M = meta
}

func (d *VirtualMachineGroupsDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Retrieve a list of all Virtual Machine Groups " +
			"in the organization.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "Always set to the " +
					"organization sub-domain.",
			},
			"groups": schema.ListNestedAttribute{
				Computed: true,
				MarkdownDescription: "A list of all Virtual Machine Groups " +
					"in the organization.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed: true,
							MarkdownDescription: "The unique identifier of " +
								"the Virtual Machine Group.",
						},
						"name": schema.StringAttribute{
							Computed: true,
							MarkdownDescription: "The name of the " +
								"Virtual Machine Group.",
						},
						"segregate": schema.BoolAttribute{
							Computed: true,
							MarkdownDescription: "Whether Virtual Machines " +
								"in this group are segregated across " +
								"separate host machines.",
						},
					},
				},
			},
		},
	}
}

func (d *VirtualMachineGroupsDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var data VirtualMachineGroupsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	res, err := d.M.Core.GetOrganizationVirtualMachineGroupsWithResponse(ctx,
		&core.GetOrganizationVirtualMachineGroupsParams{
			OrganizationSubDomain: &d.M.confOrganization,
		})
	if err != nil {
		if res != nil {
			err = genericAPIError(err, res.Body)
		}
		resp.Diagnostics.AddError("Read Error", err.Error())
		return
	}

	groups := make(
		[]VirtualMachineGroupDataSourceModel,
		0,
		len(res.JSON200.VirtualMachineGroups),
	)
	for _, vmg := range res.JSON200.VirtualMachineGroups {
		groups = append(groups, VirtualMachineGroupDataSourceModel{
			ID:        types.StringPointerValue(vmg.Id),
			Name:      types.StringPointerValue(vmg.Name),
			Segregate: types.BoolPointerValue(vmg.Segregate),
		})
	}

	data.ID = types.StringValue(d.M.confOrganization)
	data.Groups = groups

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
