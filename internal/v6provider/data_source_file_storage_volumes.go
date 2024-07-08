package v6provider

import (
	"context"
	"fmt"

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
	FileStorageVolumesDataSource struct {
		M *Meta
	}

	FileStorageVolumesDataSourceModel struct {
		ID                 types.String `tfsdk:"id"`
		FileStorageVolumes types.List   `tfsdk:"file_storage_volumes"`
	}
)

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
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "Always set to provider " +
					"organization value.",
			},
			"file_storage_volumes": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "A list of file storage volumes.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed: true,
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
							MarkdownDescription: "The resource IDs " +
								"which can access " +
								"this file storage volume. " +
								"Currently only accepts " +
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
							MarkdownDescription: "The NFS location " +
								"indicating where " +
								"to mount the volume from. This is " +
								"where the volume " +
								"must be mounted from inside " +
								"of virtual machines " +
								"referenced in `associations`.",
						},
						"size": schema.Int64Attribute{
							Computed: true,
							MarkdownDescription: "The size of the " +
								"volume in bytes.",
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
	volumes := []attr.Value{}
	totalPages := 2

	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		res, err := r.M.Core.GetOrganizationFileStorageVolumesWithResponse(ctx,
			&core.GetOrganizationFileStorageVolumesParams{
				OrganizationSubDomain: &r.M.confOrganization,
				Page:                  &pageNum,
			})
		if err != nil {
			resp.Diagnostics.AddError(
				"FileStorageVolumes Error",
				err.Error(),
			)

			return
		}

		totalPages, _ = res.JSON200.Pagination.TotalPages.Get()

		for _, fsv := range res.JSON200.FileStorageVolumes {
			fmt.Println(*fsv.Name)
			associations := []attr.Value{}
			NFSLocation, _ := fsv.NfsLocation.Get()
			Size, _ := fsv.Size.Get()
			vol := types.ObjectValueMust(
				map[string]attr.Type{
					"id":   types.StringType,
					"name": types.StringType,
					"associations": types.SetType{
						ElemType: types.StringType,
					},
					"nfs_location": types.StringType,
					"size":         types.Int64Type,
				},
				map[string]attr.Value{
					"id":   types.StringPointerValue(fsv.Id),
					"name": types.StringPointerValue(fsv.Name),
					"associations": types.SetValueMust(
						types.StringType,
						associations,
					),
					"nfs_location": types.StringValue(NFSLocation),
					"size":         types.Int64Value(int64(Size)),
				},
			)

			volumes = append(volumes, vol)
		}
	}

	resp.Diagnostics.Append(
		resp.State.Set(ctx, &FileStorageVolumesDataSourceModel{
			ID: types.StringValue(r.M.confOrganization),
			FileStorageVolumes: types.ListValueMust(
				types.ObjectType{
					AttrTypes: map[string]attr.Type{
						"id":   types.StringType,
						"name": types.StringType,
						"associations": types.SetType{
							ElemType: types.StringType,
						},
						"nfs_location": types.StringType,
						"size":         types.Int64Type,
					},
				},
				volumes,
			),
		})...)
}
