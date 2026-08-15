package v6provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

type (
	DiskIOProfilesDataSource struct {
		M *Meta
	}

	DiskIOProfilesDataSourceModel struct {
		Profiles []DiskIOProfileDataSourceModel `tfsdk:"profiles"`
	}
)

func (d *DiskIOProfilesDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_disk_io_profiles"
}

func (d *DiskIOProfilesDataSource) Configure(
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

func (d *DiskIOProfilesDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists disk I/O profiles available to the provider organization.",
		Attributes: map[string]schema.Attribute{
			"profiles": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Disk I/O profiles ordered lexically by ID.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: diskIOProfileSchemaAttributes(false),
				},
			},
		},
	}
}

func (d *DiskIOProfilesDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var data DiskIOProfilesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	profiles, err := fetchAllOrganizationDiskIOProfiles(ctx, d.M)
	if err != nil {
		resp.Diagnostics.AddError("Disk I/O Profiles Error", err.Error())
		return
	}

	data.Profiles = make([]DiskIOProfileDataSourceModel, len(profiles))
	for i := range profiles {
		data.Profiles[i] = diskIOProfileDataSourceModel(&profiles[i])
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
