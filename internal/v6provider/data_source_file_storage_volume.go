package v6provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
)

type (
	FileStorageVolumeDataSource struct {
		M *Meta
	}

	FileStorageVolumeDataSourceModel struct {
		ID           types.String `tfsdk:"id"`
		Name         types.String `tfsdk:"name"`
		Associations types.Set    `tfsdk:"associations"`
		NFSLocation  types.String `tfsdk:"nfs_location"`
		Size         types.Int64  `tfsdk:"size"`
	}
)

func (r FileStorageVolumeDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_file_storage_volume"
}

func (r *FileStorageVolumeDataSource) Configure(
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

func (r *FileStorageVolumeDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The ID of the file storage volume.",
			},
			"name": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "Unique name to help " +
					"identify the volume. " +
					"Must be unique within the organization.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"associations": schema.SetAttribute{
				Computed: true,
				MarkdownDescription: "The resource IDs which can access " +
					"this file storage volume. Currently only accepts " +
					"virtual machine IDs.",
				ElementType: types.StringType,
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(
						stringvalidator.LengthAtLeast(1),
					),
				},
			},
			"nfs_location": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "The NFS location indicating where " +
					"to mount the volume from. This is where the volume " +
					"must be mounted from inside of virtual machines " +
					"referenced in `associations`.",
			},
			"size": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The size of the volume in bytes.",
			},
		},
	}
}

func (r *FileStorageVolumeDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var data FileStorageVolumeDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	res, err := r.M.Core.GetFileStorageVolumeWithResponse(ctx,
		&core.GetFileStorageVolumeParams{
			FileStorageVolumeId: data.ID.ValueStringPointer(),
		})
	if err != nil {
		resp.Diagnostics.AddError(
			"File Storage Volume Error",
			err.Error(),
		)
		return
	}

	fsv := res.JSON200.FileStorageVolume

	data.Name = types.StringPointerValue(fsv.Name)

	NFSLocation, _ := fsv.NfsLocation.Get()
	data.NFSLocation = types.StringValue(NFSLocation)

	Size, _ := fsv.Size.Get()
	data.Size = types.Int64Value(int64(Size))

	associations := []attr.Value{}
	for _, a := range *fsv.Associations {
		associations = append(associations, types.StringValue(a))
	}

	data.Associations = types.SetValueMust(types.StringType, associations)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
