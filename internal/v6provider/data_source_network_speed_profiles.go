package v6provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type (
	NetworkSpeedProfilesDataSource struct {
		M *Meta
	}

	NetworkSpeedProfilesDataSourceModel struct {
		ID       types.String                         `tfsdk:"id"`
		Profiles []NetworkSpeedProfileDataSourceModel `tfsdk:"profiles"`
	}
)

func (d *NetworkSpeedProfilesDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_network_speed_profiles"
}

func (d *NetworkSpeedProfilesDataSource) Configure(
	_ context.Context,
	req datasource.ConfigureRequest,
	resp *datasource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		return
	}

	meta, ok := req.ProviderData.(*Meta)
	if !ok {
		resp.Diagnostics.AddError("Meta Error", "meta is not of type *Meta")
		return
	}

	d.M = meta
}

func (d *NetworkSpeedProfilesDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists network speed profiles available to the provider organization.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Always set to the provider organization value.",
			},
			"profiles": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The available network speed profiles.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: networkSpeedProfileSchemaAttributes(false),
				},
			},
		},
	}
}

func (d *NetworkSpeedProfilesDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var data NetworkSpeedProfilesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	profiles, err := fetchAllOrganizationNetworkSpeedProfiles(ctx, d.M)
	if err != nil {
		resp.Diagnostics.AddError("Network Speed Profiles Error", err.Error())
		return
	}

	data.ID = types.StringValue(d.M.confOrganization)
	data.Profiles = make([]NetworkSpeedProfileDataSourceModel, len(profiles))
	for i := range profiles {
		data.Profiles[i] = networkSpeedProfileDataSourceModel(&profiles[i])
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
