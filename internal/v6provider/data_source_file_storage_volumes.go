package v6provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
)

type FileStorageVolumesDataSource struct {
	M *Meta
}

//nolint:lll
type FileStorageVolumesDataSourceModel struct {
	FileStorageVolumes []FileStorageVolumeDataSourceModel `tfsdk:"file_storage_volumes"`
}

func (r FileStorageVolumesDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_file_storage_volumes"
}

func (r *FileStorageVolumesDataSource) Configure(
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

	r.M = meta
}

func (r *FileStorageVolumesDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description: "Fetch all file storage volumes in the organization.",
		Attributes: map[string]schema.Attribute{
			"file_storage_volumes": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "A list of file storage volumes.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed: true,
							MarkdownDescription: "The ID of the file storage " +
								"volume.",
						},
						"name": schema.StringAttribute{
							Computed: true,
							MarkdownDescription: "Unique name to help " +
								"identify the volume. Must be unique within " +
								"the organization.",
						},
						"associations": schema.SetAttribute{
							Computed: true,
							MarkdownDescription: "The resource IDs which can " +
								"access this file storage volume. Currently " +
								"only accepts virtual machine IDs.",
							ElementType: types.StringType,
						},
						"nfs_location": schema.StringAttribute{
							Computed: true,
							MarkdownDescription: "The NFS location " +
								"indicating where to mount the volume from. " +
								"This is where the volume must be mounted " +
								"from inside of virtual machines referenced " +
								"in `associations`.",
						},
						"size": schema.Int64Attribute{
							Computed: true,
							MarkdownDescription: "The size of the volume in " +
								"bytes.",
						},
					},
				},
			},
		},
	}
}

func (r *FileStorageVolumesDataSource) Read(
	ctx context.Context,
	_ datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var data FileStorageVolumesDataSourceModel
	var volumes []FileStorageVolumeDataSourceModel
	totalPages := 2

	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		res, err := r.M.Core.GetOrganizationFileStorageVolumesWithResponse(ctx,
			&core.GetOrganizationFileStorageVolumesParams{
				OrganizationSubDomain: &r.M.confOrganization,
				Page:                  &pageNum,
			})
		if err != nil {
			if res != nil {
				err = genericAPIError(err, res.Body)
			}

			resp.Diagnostics.AddError("File Storage Volumes Error", err.Error())
			return
		}

		totalPages, _ = res.JSON200.Pagination.TotalPages.Get()

		for _, fsv := range res.JSON200.FileStorageVolumes {
			vol := FileStorageVolumeDataSourceModel{
				ID:   types.StringPointerValue(fsv.Id),
				Name: types.StringPointerValue(fsv.Name),
			}

			if v, err := fsv.NfsLocation.Get(); err == nil {
				vol.NFSLocation = types.StringValue(v)
			}

			if v, err := fsv.Size.Get(); err == nil {
				vol.Size = types.Int64Value(int64(v))
			}

			elements, diags := types.SetValueFrom(
				ctx, types.StringType, fsv.Associations,
			)
			resp.Diagnostics.Append(diags...)
			if resp.Diagnostics.HasError() {
				return
			}

			vol.Associations = elements

			volumes = append(volumes, vol)
		}
	}

	data.FileStorageVolumes = volumes
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
