package v6provider

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/krystal/go-katapult/next/core"
)

type (
	FileStorageVolumeResource struct {
		M *Meta
	}

	FileStorageVolumeResourceModel struct {
		ID           types.String   `tfsdk:"id"`
		Name         types.String   `tfsdk:"name"`
		Associations types.Set      `tfsdk:"associations"`
		NFSLocation  types.String   `tfsdk:"nfs_location"`
		Timeouts     timeouts.Value `tfsdk:"timeouts"`
	}
)

func (r FileStorageVolumeResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_file_storage_volume"
}

func (r *FileStorageVolumeResource) Configure(
	_ context.Context,
	req resource.ConfigureRequest,
	resp *resource.ConfigureResponse,
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

func (r FileStorageVolumeResource) Schema(
	ctx context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: strings.TrimSpace(`

The File Storage Volume resource allows you to manage File Storage Volumes in Katapult.

-> **Note:** Volumes are not automatically mounted within associated virtual machines. This must be done manually or via a provisioning tool of some kind, using the ` + "`nfs_location`" + ` attribute value as the mount source.

~> **Warning:** Deleting a file storage volume resource with Terraform will by default purge the volume from Katapult's trash, permanently deleting it. If you wish to instead keep a deleted volume in the trash, set the` + "`skip_trash_object_purge`" + ` provider option to ` + "`true`" + `. By default, objects in the trash are permanently deleted after 48 hours.

`,
		),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "The ID of the file storage volume. " +
					"This is automatically generated by the API.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Unique name to help " +
					"identify the volume. " +
					"Must be unique within the organization.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"associations": schema.SetAttribute{
				Optional: true,
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
				PlanModifiers: []planmodifier.Set{
					NullToEmptySetPlanModifier(),
					setplanmodifier.UseStateForUnknown(),
				},
			},
			"nfs_location": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "The NFS location indicating where " +
					"to mount the volume from. This is where the volume " +
					"must be mounted from inside of virtual machines " +
					"referenced in `associations`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true,
				Delete: true,
			}),
		},
	}
}

func (r *FileStorageVolumeResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan FileStorageVolumeResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	associations := []string{}
	resp.Diagnostics.Append(
		plan.Associations.ElementsAs(
			ctx,
			&associations,
			false,
		)...)
	if resp.Diagnostics.HasError() {
		return
	}

	res, err := r.M.Core.PostOrganizationFileStorageVolumesWithResponse(
		ctx,
		core.PostOrganizationFileStorageVolumesJSONRequestBody{
			Organization: core.OrganizationLookup{
				SubDomain: &r.M.confOrganization,
			},
			Properties: core.FileStorageVolumeArguments{
				DataCenter: &core.DataCenterLookup{
					Permalink: &r.M.confDataCenter,
				},
				Name:         plan.Name.ValueStringPointer(),
				Associations: &associations,
			},
		},
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"FileStorageVolumeCreate Error",
			"Error creating file storage volume: "+err.Error(),
		)
		return
	}

	create, diags := plan.Timeouts.Create(ctx, 1*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	fsv, err := waitForFileStorageVolumeToBeReady(
		ctx, r.M, create, 2*time.Second, res.JSON201.FileStorageVolume.Id,
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error waiting for file storage "+
				"volume to become ready.",
			err.Error(),
		)
	}

	if err := r.FileStorageVolumeRead(
		ctx,
		fsv.Id,
		&plan,
		&resp.State,
	); err != nil {
		resp.Diagnostics.AddError(
			"FileStorageVolumeRead Error",
			"Error reading file storage volume: "+err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *FileStorageVolumeResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	state := &FileStorageVolumeResourceModel{}
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.FileStorageVolumeRead(
		ctx, state.ID.ValueStringPointer(), state, &resp.State,
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"FileStorageVolumeRead Error",
			"Error reading file storage volume: "+err.Error(),
		)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *FileStorageVolumeResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan FileStorageVolumeResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state FileStorageVolumeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ref := core.FileStorageVolumeLookup{Id: state.ID.ValueStringPointer()}
	args := core.FileStorageVolumeArguments{}

	if !plan.Name.Equal(state.Name) {
		args.Name = plan.Name.ValueStringPointer()
	}

	if !plan.Associations.Equal(state.Associations) {
		associations := []string{}
		resp.Diagnostics.Append(
			plan.Associations.ElementsAs(
				ctx,
				&associations,
				false,
			)...)
		if resp.Diagnostics.HasError() {
			return
		}

		args.Associations = &associations
	}

	res, err := r.M.Core.PatchFileStorageVolumeWithResponse(ctx,
		core.PatchFileStorageVolumeJSONRequestBody{
			FileStorageVolume: ref,
			Properties:        args,
		})
	if err != nil {
		resp.Diagnostics.AddError(
			"FileStorageVolumeUpdate Error",
			"Error updating file storage volume: "+err.Error(),
		)
		return
	}

	fsv := res.JSON200.FileStorageVolume

	_, err = waitForFileStorageVolumeToBeReady(
		ctx,
		r.M,
		20*time.Minute,
		5*time.Second,
		fsv.Id)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error waiting for file storage "+
				"volume to become ready.",
			err.Error(),
		)
	}

	if err := r.FileStorageVolumeRead(
		ctx,
		fsv.Id,
		&plan,
		&resp.State,
	); err != nil {
		resp.Diagnostics.AddError(
			"FileStorageVolumeRead Error",
			"Error reading file storage volume: "+err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

//nolint:funlen // only a few more lines than the max
func (r *FileStorageVolumeResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state FileStorageVolumeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteTime, diags := state.Timeouts.Delete(ctx, 2*time.Minute)

	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	res, err := r.M.Core.GetFileStorageVolumeWithResponse(ctx,
		&core.GetFileStorageVolumeParams{
			FileStorageVolumeId: state.ID.ValueStringPointer(),
		})
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return
		} else if errors.Is(err, core.ErrRequestFailed) &&
			*res.JSON406.Code == core.ObjectInTrashEnumObjectInTrash {
			if r.M.SkipTrashObjectPurge {
				return
			}

			purgeError := purgeTrashObjectByObjectID(
				ctx, r.M, deleteTime, state.ID.ValueString(),
			)
			if purgeError != nil {
				resp.Diagnostics.AddError(
					"Failed to purge file storage volume from trash.",
					purgeError.Error(),
				)

				return
			}

			return
		}

		resp.Diagnostics.AddError(
			"Failed to lookup file storage volume details.",
			err.Error(),
		)

		return
	}

	fsv := res.JSON200.FileStorageVolume

	// If we're skipping purge, we rename the file storage volume before
	// deletion to include its ID. This allows it to be easily identified in the
	// trash, and also avoids name conflicts if another volume is created with
	// the same name.
	if r.M.SkipTrashObjectPurge {
		// Append the ID to the end of the name, if it's not already there. If
		// the resulting name would be too long, truncate name to fit once the
		// ID is appended.
		name := *fsv.Name
		suffix := "-" + *fsv.Id
		if !strings.HasSuffix(name, suffix) {
			if len(name)+len(suffix) > 128 {
				name = name[:128-len(suffix)]
			}
			name += suffix
		}

		res, patchErr := r.M.Core.PatchFileStorageVolumeWithResponse(ctx,
			core.PatchFileStorageVolumeJSONRequestBody{
				FileStorageVolume: core.FileStorageVolumeLookup{Id: fsv.Id},
				Properties:        core.FileStorageVolumeArguments{Name: &name},
			})

		if patchErr != nil && !isErrNotFoundOrInTrash(patchErr, res.JSON406) {
			resp.Diagnostics.AddError("Failed to rename file storage "+
				"volume before moving to trash.",
				patchErr.Error())

			return
		}
	}

	delRes, err := r.M.Core.DeleteFileStorageVolumeWithResponse(ctx,
		core.DeleteFileStorageVolumeJSONRequestBody{
			FileStorageVolume: core.FileStorageVolumeLookup{Id: fsv.Id},
		})

	if err != nil && !isErrNotFoundOrInTrash(err, delRes.JSON406) {
		resp.Diagnostics.AddError("FileStorageVolumeDelete Error", err.Error())
		return
	}

	if !r.M.SkipTrashObjectPurge {
		err = purgeTrashObjectByObjectID(
			ctx, r.M, deleteTime, *fsv.Id,
		)
		if err != nil {
			resp.Diagnostics.AddError(
				"Failed to purge file storage volume from trash.",
				err.Error(),
			)
			return
		}
	}
}

func (r *FileStorageVolumeResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *FileStorageVolumeResource) FileStorageVolumeRead(
	ctx context.Context,
	id *string,
	model *FileStorageVolumeResourceModel,
	state *tfsdk.State,
) error {
	res, err := r.M.Core.GetFileStorageVolumeWithResponse(ctx,
		&core.GetFileStorageVolumeParams{
			FileStorageVolumeId: id,
		})
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			state.RemoveResource(ctx)

			return nil
		}

		return err
	}

	fsv := res.JSON200.FileStorageVolume

	model.ID = types.StringPointerValue(fsv.Id)
	model.Name = types.StringPointerValue(fsv.Name)

	NFSLocation, _ := fsv.NfsLocation.Get()
	model.NFSLocation = types.StringValue(NFSLocation)

	if fsv.Associations != nil && len(*fsv.Associations) > 0 {
		associations := []attr.Value{}
		for _, a := range *fsv.Associations {
			associations = append(associations, types.StringValue(a))
		}

		model.Associations = types.SetValueMust(types.StringType, associations)
	}

	return nil
}

// Helper

func waitForFileStorageVolumeToBeReady(
	ctx context.Context,
	m *Meta,
	timeout time.Duration,
	delay time.Duration,
	fsvID *string,
) (*core.GetFileStorageVolume200ResponseFileStorageVolume, error) {
	waiter := &retry.StateChangeConf{
		Pending: []string{
			string(core.FileStorageVolumeStateEnumPending),
			string(core.FileStorageVolumeStateEnumConfiguring),
		},
		Target: []string{
			string(core.FileStorageVolumeStateEnumReady),
		},
		Refresh: func() (interface{}, string, error) {
			res, err := m.Core.GetFileStorageVolumeWithResponse(ctx,
				&core.GetFileStorageVolumeParams{
					FileStorageVolumeId: fsvID,
				})
			if err != nil {
				return nil, "", err
			}

			f := &res.JSON200.FileStorageVolume

			return f, string(*f.State), nil
		},
		Timeout:                   timeout,
		Delay:                     delay,
		MinTimeout:                5 * time.Second,
		ContinuousTargetOccurence: 1,
	}

	readyFSV, err := waiter.WaitForStateContext(ctx)

	if readyFSV == nil {
		return nil, err
	}

	//nolint:lll // Generated type names are long.
	return readyFSV.(*core.GetFileStorageVolume200ResponseFileStorageVolume), err
}
