package v6provider

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
)

type (
	DiskResource struct {
		M *Meta
	}

	DiskResourceModel struct {
		ID           types.String `tfsdk:"id"`
		Name         types.String `tfsdk:"name"`
		SizeInGB     types.Int64  `tfsdk:"size_in_gb"`
		StorageSpeed types.String `tfsdk:"storage_speed"`
		BusType      types.String `tfsdk:"bus_type"`
		IOProfileID  types.String `tfsdk:"io_profile_id"`
		ResizeMethod types.String `tfsdk:"resize_method"`
		WWN          types.String `tfsdk:"wwn"`
		State        types.String `tfsdk:"state"`
	}
)

func (r *DiskResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_disk"
}

func (r *DiskResource) Configure(
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

func (r *DiskResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a standalone disk in Katapult.\n\n" +
			"Destroying a `katapult_virtual_machine` only detaches its " +
			"attached disks — it does **not** delete them. The disk is " +
			"deleted only when this resource itself is destroyed. Use " +
			"`lifecycle { prevent_destroy = true }` to guard important data.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The unique identifier of the disk.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the disk.",
			},
			"size_in_gb": schema.Int64Attribute{
				Required: true,
				MarkdownDescription: "Size of the disk in GB. " +
					"Can only be increased; decreasing triggers replacement.",
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
				PlanModifiers: []planmodifier.Int64{
					RequiresReplaceIfDecreased(),
				},
			},
			"storage_speed": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Storage speed for the disk: " +
					"`ssd` or `nvme`. Cannot be changed after creation " +
					"(requires replacement).\n\n" +
					"~> **Note:** Available storage tiers vary by data " +
					"center. For portable configurations, omit this " +
					"attribute and the data center's default tier will " +
					"be used.",
				Validators: []validator.String{
					stringvalidator.OneOf("ssd", "nvme"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"bus_type": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Bus type for the disk: " +
					"`virtio` or `scsi`.",
				Validators: []validator.String{
					stringvalidator.OneOf("virtio", "scsi"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"io_profile_id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "The ID of the IO profile to apply.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"resize_method": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Resize method when growing the disk: " +
					"`online` or `offline`. Write-only — not returned by the API.",
				Validators: []validator.String{
					stringvalidator.OneOf("online", "offline"),
				},
			},
			"wwn": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "World Wide Name identifier of the disk.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Current state of the disk.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *DiskResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan DiskResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeout := 10 * time.Minute

	name := plan.Name.ValueString()
	sizeInGB := int(plan.SizeInGB.ValueInt64())

	args := core.DiskArguments{
		Name:     &name,
		SizeInGb: &sizeInGB,
		DataCenter: &core.DataCenterLookup{
			Permalink: &r.M.confDataCenter,
		},
	}

	if !plan.StorageSpeed.IsNull() && !plan.StorageSpeed.IsUnknown() {
		ss := core.StorageSpeedEnum(plan.StorageSpeed.ValueString())
		args.StorageSpeed = &ss
	}
	if !plan.BusType.IsNull() && !plan.BusType.IsUnknown() {
		bt := core.DiskBusEnum(plan.BusType.ValueString())
		args.BusType = &bt
	}
	if !plan.IOProfileID.IsNull() && !plan.IOProfileID.IsUnknown() {
		ioID := plan.IOProfileID.ValueString()
		args.IoProfile = &core.DiskIOProfileLookup{Id: &ioID}
	}

	createRes, err := r.M.Core.PostOrganizationDisksWithResponse(ctx,
		core.PostOrganizationDisksJSONRequestBody{
			Organization: core.OrganizationLookup{
				SubDomain: &r.M.confOrganization,
			},
			Properties: args,
		})
	if err != nil {
		if createRes != nil {
			err = genericAPIError(err, createRes.Body)
		}
		resp.Diagnostics.AddError("Create Error", err.Error())
		return
	}
	if createRes.JSON201 == nil {
		resp.Diagnostics.AddError("Create Error",
			"unexpected empty response creating disk")
		return
	}

	diskID := *createRes.JSON201.Disk.Id
	taskID := *createRes.JSON201.Task.Id

	// Persist the disk ID immediately so state is preserved if poll times out.
	plan.ID = types.StringValue(diskID)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := waitForTaskCompletion(ctx, r.M, timeout, taskID); err != nil {
		resp.Diagnostics.AddError("Create Error",
			fmt.Sprintf("error waiting for disk creation: %s", err))
		return
	}

	if err := r.diskRead(ctx, diskID, &plan); err != nil {
		resp.Diagnostics.AddError("Read Error", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *DiskResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state DiskResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.diskRead(ctx, state.ID.ValueString(), &state)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read Error", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *DiskResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan DiskResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state DiskResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeout := 10 * time.Minute
	diskID := state.ID.ValueString()

	if err := r.patchDiskProperties(ctx, diskID, &plan, &state); err != nil {
		resp.Diagnostics.AddError("Update Error", err.Error())
		return
	}

	if err := r.resizeDisk(ctx, diskID, &plan, &state, timeout); err != nil {
		resp.Diagnostics.AddError("Update Error", err.Error())
		return
	}

	if err := r.diskRead(ctx, diskID, &plan); err != nil {
		resp.Diagnostics.AddError("Read Error", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// patchDiskProperties applies any name / bus_type / io_profile_id changes via
// PatchDisk. No-op when none of those attributes have changed.
func (r *DiskResource) patchDiskProperties(
	ctx context.Context,
	diskID string,
	plan, state *DiskResourceModel,
) error {
	if plan.Name.Equal(state.Name) &&
		plan.BusType.Equal(state.BusType) &&
		plan.IOProfileID.Equal(state.IOProfileID) {
		return nil
	}

	patchArgs := core.DiskArguments{}
	if !plan.Name.Equal(state.Name) {
		name := plan.Name.ValueString()
		patchArgs.Name = &name
	}
	if !plan.BusType.Equal(state.BusType) && !plan.BusType.IsNull() {
		bt := core.DiskBusEnum(plan.BusType.ValueString())
		patchArgs.BusType = &bt
	}
	if !plan.IOProfileID.Equal(state.IOProfileID) && !plan.IOProfileID.IsNull() {
		ioID := plan.IOProfileID.ValueString()
		patchArgs.IoProfile = &core.DiskIOProfileLookup{Id: &ioID}
	}

	patchRes, err := r.M.Core.PatchDiskWithResponse(ctx,
		core.PatchDiskJSONRequestBody{
			Disk:       core.DiskLookup{Id: &diskID},
			Properties: patchArgs,
		})
	if err != nil {
		if patchRes != nil {
			return genericAPIError(err, patchRes.Body)
		}
		return err
	}

	return nil
}

// resizeDisk grows the disk via PutDiskResize when size_in_gb changes,
// waiting on the returned task. No-op when size hasn't changed.
func (r *DiskResource) resizeDisk(
	ctx context.Context,
	diskID string,
	plan, state *DiskResourceModel,
	timeout time.Duration,
) error {
	if plan.SizeInGB.Equal(state.SizeInGB) {
		return nil
	}

	newSize := int(plan.SizeInGB.ValueInt64())

	var resizeMethod *core.ResizeMethodEnum
	if !plan.ResizeMethod.IsNull() && !plan.ResizeMethod.IsUnknown() {
		rm := core.ResizeMethodEnum(plan.ResizeMethod.ValueString())
		resizeMethod = &rm
	}

	resizeRes, err := r.M.Core.PutDiskResizeWithResponse(ctx,
		core.PutDiskResizeJSONRequestBody{
			Disk:         core.DiskLookup{Id: &diskID},
			SizeInGb:     newSize,
			ResizeMethod: resizeMethod,
		})
	if err != nil {
		if resizeRes != nil {
			return genericAPIError(err, resizeRes.Body)
		}
		return err
	}
	if resizeRes.JSON200 == nil || resizeRes.JSON200.Task.Id == nil {
		return fmt.Errorf("unexpected empty response resizing disk")
	}

	return waitForTaskCompletion(
		ctx, r.M, timeout, *resizeRes.JSON200.Task.Id,
	)
}

func (r *DiskResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state DiskResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeout := 5 * time.Minute
	diskID := state.ID.ValueString()

	// Check if the disk is currently assigned to a VM and detach if so.
	diskRes, err := r.M.Core.GetDiskWithResponse(ctx,
		&core.GetDiskParams{DiskId: &diskID})
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return
		}
		if diskRes != nil {
			if isErrNotFoundOrInTrash(err, diskRes.JSON406) {
				return
			}
			err = genericAPIError(err, diskRes.Body)
		}
		resp.Diagnostics.AddError("Delete Error", err.Error())
		return
	}
	if diskRes.JSON200 != nil {
		disk := diskRes.JSON200.Disk
		if disk.VirtualMachineDisk.IsSpecified() {
			if e := detachAndUnassignDisk(ctx, r.M, diskID, timeout); e != nil {
				resp.Diagnostics.AddError("Delete Error", e.Error())
				return
			}
		}
	}

	delRes, err := r.M.Core.DeleteDiskWithResponse(ctx,
		core.DeleteDiskJSONRequestBody{
			Disk: core.DiskLookup{Id: &diskID},
		})
	if err != nil {
		if delRes != nil {
			if isErrNotFoundOrInTrash(err, delRes.JSON406) {
				return
			}
			err = genericAPIError(err, delRes.Body)
		}
		resp.Diagnostics.AddError("Delete Error", err.Error())
		return
	}

	if !r.M.SkipTrashObjectPurge && delRes != nil && delRes.JSON200 != nil {
		trashObj := delRes.JSON200.TrashObject
		if e := purgeTrashObject(ctx, r.M, timeout, trashObj); e != nil &&
			!isErrNotFoundOrInTrash(e, nil) {
			resp.Diagnostics.AddError("Delete Error",
				fmt.Sprintf("failed to purge disk from trash: %s", e))
		}
	}
}

func (r *DiskResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *DiskResource) diskRead(
	ctx context.Context,
	id string,
	model *DiskResourceModel,
) error {
	res, err := r.M.Core.GetDiskWithResponse(ctx,
		&core.GetDiskParams{DiskId: &id})
	if err != nil {
		if res != nil {
			if isErrNotFoundOrInTrash(err, res.JSON406) {
				return core.ErrNotFound
			}
			return genericAPIError(err, res.Body)
		}
		return err
	}
	if res.JSON200 == nil {
		return fmt.Errorf("unexpected empty response fetching disk")
	}

	disk := res.JSON200.Disk

	if disk.Name != nil {
		model.Name = types.StringValue(*disk.Name)
	}
	if disk.SizeInGb != nil {
		model.SizeInGB = types.Int64Value(int64(*disk.SizeInGb))
	}
	if disk.StorageSpeed != nil {
		model.StorageSpeed = types.StringValue(string(*disk.StorageSpeed))
	}
	if disk.BusType.IsSpecified() {
		if bt, e := disk.BusType.Get(); e == nil {
			model.BusType = types.StringValue(string(bt))
		}
	}
	if disk.IoProfile.IsSpecified() {
		if iop, e := disk.IoProfile.Get(); e == nil && iop.Id != nil {
			model.IOProfileID = types.StringValue(*iop.Id)
		}
	} else if !model.IOProfileID.IsNull() {
		model.IOProfileID = types.StringNull()
	}
	if disk.Wwn != nil {
		model.WWN = types.StringValue(*disk.Wwn)
	}
	if disk.State != nil {
		model.State = types.StringValue(string(*disk.State))
	}
	// ResizeMethod is write-only — do not overwrite from API response.

	return nil
}
