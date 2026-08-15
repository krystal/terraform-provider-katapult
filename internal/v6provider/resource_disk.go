package v6provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
)

const diskResizeMethodPrivateKey = "disk_resize_method_v1"

const diskMarkdownDescription = "Manages a standalone disk in Katapult.\n\n" +
	"Assignment lifecycle is owned by `katapult_disk_assignment`. This resource " +
	"refuses deletion while any assignment remains and never detaches or unassigns " +
	"a relationship itself. Remove the assignment first so Terraform's dependency " +
	"graph orders detach and unassign before disk deletion. The disk is deleted only " +
	"when this resource itself is destroyed; use `lifecycle { prevent_destroy = true }` " +
	"to guard important data.\n\n" +
	"Offline resize requires the disk to be physically detached. Because an in-place " +
	"disk resize and an in-place `katapult_disk_assignment.attached = false` update " +
	"cannot be ordered safely in one Terraform graph, perform them in two applies: " +
	"detach first, then change `size_in_gb`. Online growth leaves guest partition and " +
	"filesystem expansion to the operator. Shrink is always offline and may require a " +
	"larger update timeout."

type (
	DiskResource struct {
		M *Meta
	}

	DiskResourceModel struct {
		ID           types.String   `tfsdk:"id"`
		Name         types.String   `tfsdk:"name"`
		SizeInGB     types.Int64    `tfsdk:"size_in_gb"`
		StorageSpeed types.String   `tfsdk:"storage_speed"`
		BusType      types.String   `tfsdk:"bus_type"`
		IOProfileID  types.String   `tfsdk:"io_profile_id"`
		ResizeMethod types.String   `tfsdk:"resize_method"`
		WWN          types.String   `tfsdk:"wwn"`
		State        types.String   `tfsdk:"state"`
		Timeouts     timeouts.Value `tfsdk:"timeouts"`
	}
)

var _ resource.ResourceWithModifyPlan = (*DiskResource)(nil)

//nolint:lll // Keep complete operator-facing resize guidance next to each diagnostic.
func (r *DiskResource) ModifyPlan(
	ctx context.Context,
	req resource.ModifyPlanRequest,
	resp *resource.ModifyPlanResponse,
) {
	if r.M == nil || req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	var plan, state DiskResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || plan.SizeInGB.IsUnknown() ||
		state.SizeInGB.IsNull() || plan.SizeInGB.Equal(state.SizeInGB) {
		return
	}
	obs, err := readStandaloneDiskRelationship(ctx, r.M, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("size_in_gb"), "Cannot Validate Disk Resize", err.Error())
		return
	}
	method, err := effectiveDiskResizeMethod(
		state.SizeInGB.ValueInt64(), plan.SizeInGB.ValueInt64(),
		plan.ResizeMethod.ValueString(), obs.assigned, obs.attachmentState,
	)
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("size_in_gb"), "Invalid Disk Resize", err.Error())
		return
	}
	if method == core.Online {
		resp.Diagnostics.AddAttributeWarning(path.Root("size_in_gb"), "Online Disk Growth", "Katapult grows the block device only; expand the guest partition and filesystem manually.")
	}
	if plan.SizeInGB.ValueInt64() < state.SizeInGB.ValueInt64() {
		resp.Diagnostics.AddAttributeWarning(path.Root("size_in_gb"), "Offline Disk Shrink", "Katapult must shrink the filesystem and partition before the disk. Insufficient free capacity fails the task, and large disks may need a longer update timeout.")
	}
	encodedMethod, err := json.Marshal(string(method))
	if err != nil {
		resp.Diagnostics.AddError("Disk Resize Plan Error", err.Error())
		return
	}
	if resp.Private != nil {
		resp.Diagnostics.Append(resp.Private.SetKey(ctx, diskResizeMethodPrivateKey, encodedMethod)...)
	}
}

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

//nolint:goconst // Terraform attribute names are clearer inline in schema declarations.
func (r *DiskResource) Schema(
	ctx context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: diskMarkdownDescription,
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
				MarkdownDescription: "Size of the disk in GB. Growth and shrink " +
					"are performed in place when the configured resize method and " +
					"physical attachment state permit it.",
				Validators: []validator.Int64{
					int64validator.AtLeast(10),
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
				Computed: true,
				Default:  stringdefault.StaticString("offline"),
				MarkdownDescription: "Preferred method for growing the disk: " +
					"`online` or `offline`. Defaults to filesystem-aware offline " +
					"resizing. Shrinks and detached growth always use offline.",
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
			stateAttributeName: schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Current state of the disk.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
		Blocks: map[string]schema.Block{
			"timeouts": timeouts.Block(ctx, timeouts.Opts{
				Create: true,
				Update: true,
				Delete: true,
			}),
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

	timeout, diags := plan.Timeouts.Create(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

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
	if createRes == nil || createRes.JSON201 == nil {
		resp.Diagnostics.AddError("Create Error",
			"unexpected empty response creating disk")
		return
	}
	if createRes.JSON201.Disk.Id == nil {
		resp.Diagnostics.AddError("Create Error",
			"unexpected response creating disk: missing disk ID")
		return
	}
	if createRes.JSON201.Task.Id == nil {
		resp.Diagnostics.AddError("Create Error",
			"unexpected response creating disk: missing task ID")
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

	timeout, diags := plan.Timeouts.Update(ctx, 2*time.Hour)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	diskID := state.ID.ValueString()
	plannedResizeMethod := core.ResizeMethodEnum("")
	if req.Private != nil {
		encodedMethod, privateDiags := req.Private.GetKey(ctx, diskResizeMethodPrivateKey)
		resp.Diagnostics.Append(privateDiags...)
		if len(encodedMethod) > 0 {
			var method string
			if err := json.Unmarshal(encodedMethod, &method); err != nil {
				resp.Diagnostics.AddError("Invalid Disk Resize Plan", err.Error())
				return
			}
			plannedResizeMethod = core.ResizeMethodEnum(method)
		}
	}
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.patchDiskProperties(ctx, diskID, &plan, &state); err != nil {
		resp.Diagnostics.AddError("Update Error", err.Error())
		return
	}

	if err := r.resizeDisk(ctx, diskID, &plan, &state, plannedResizeMethod, timeout); err != nil {
		observed := plan
		refreshCtx, cancelRefresh := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancelRefresh()
		if readErr := r.diskRead(refreshCtx, diskID, &observed); readErr == nil {
			resp.Diagnostics.Append(resp.State.Set(ctx, observed)...)
		} else {
			resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
		}
		resp.Diagnostics.AddError("Update Error", err.Error())
		return
	}

	if err := r.diskRead(ctx, diskID, &plan); err != nil {
		resp.Diagnostics.AddError("Read Error", err.Error())
		return
	}
	if resp.Private != nil {
		resp.Diagnostics.Append(resp.Private.SetKey(ctx, diskResizeMethodPrivateKey, nil)...)
	}
	if resp.Diagnostics.HasError() {
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
	if patchRes == nil || patchRes.JSON200 == nil {
		return fmt.Errorf("unexpected empty response updating disk")
	}

	return nil
}

// resizeDisk grows the disk via PutDiskResize when size_in_gb changes,
// waiting on the returned task. No-op when size hasn't changed.
//
//nolint:lll // Preserve the complete plan-staleness diagnostic.
func (r *DiskResource) resizeDisk(
	ctx context.Context,
	diskID string,
	plan, state *DiskResourceModel,
	plannedMethod core.ResizeMethodEnum,
	timeout time.Duration,
) error {
	if plan.SizeInGB.Equal(state.SizeInGB) {
		return nil
	}

	newSize := int(plan.SizeInGB.ValueInt64())
	obs, err := readStandaloneDiskRelationship(ctx, r.M, diskID)
	if err != nil {
		return fmt.Errorf("rechecking disk attachment before resize: %w", err)
	}
	resizeMethod, err := effectiveDiskResizeMethod(
		state.SizeInGB.ValueInt64(), plan.SizeInGB.ValueInt64(),
		plan.ResizeMethod.ValueString(), obs.assigned, obs.attachmentState,
	)
	if err != nil {
		return err
	}
	if plannedMethod != "" && plannedMethod != resizeMethod {
		return fmt.Errorf("disk attachment state changed after planning: resize was planned as %s but now requires %s; run terraform plan again", plannedMethod, resizeMethod)
	}

	resizeRes, err := r.M.Core.PutDiskResizeWithResponse(ctx,
		core.PutDiskResizeJSONRequestBody{
			Disk:         core.DiskLookup{Id: &diskID},
			SizeInGb:     newSize,
			ResizeMethod: &resizeMethod,
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

	if err := waitForTaskCompletion(ctx, r.M, timeout, *resizeRes.JSON200.Task.Id); err != nil {
		return err
	}
	return waitForDiskSize(ctx, r.M, diskID, int64(newSize), timeout)
}

//nolint:lll // Preserve the actionable relationship ownership diagnostic.
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

	timeout, diags := state.Timeouts.Delete(ctx, 5*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	diskID := state.ID.ValueString()

	// Relationship teardown belongs exclusively to katapult_disk_assignment.
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
		if disk.VirtualMachineDisk.IsSpecified() && !disk.VirtualMachineDisk.IsNull() {
			resp.Diagnostics.AddError("Delete Error", fmt.Sprintf(
				"disk %s still has a Virtual Machine assignment; remove the corresponding katapult_disk_assignment first",
				diskID,
			))
			return
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
	model.BusType = types.StringNull()
	if disk.BusType.IsSpecified() && !disk.BusType.IsNull() {
		if bt, e := disk.BusType.Get(); e == nil {
			model.BusType = types.StringValue(string(bt))
		}
	}
	model.IOProfileID = types.StringNull()
	if disk.IoProfile.IsSpecified() && !disk.IoProfile.IsNull() {
		if iop, e := disk.IoProfile.Get(); e == nil && iop.Id != nil {
			model.IOProfileID = types.StringValue(*iop.Id)
		}
	}
	if disk.Wwn != nil {
		model.WWN = types.StringValue(*disk.Wwn)
	}
	if disk.State != nil {
		model.State = types.StringValue(string(*disk.State))
	}
	// ResizeMethod is write-only — do not overwrite from API response.
	if model.ResizeMethod.IsNull() || model.ResizeMethod.IsUnknown() {
		model.ResizeMethod = types.StringValue("offline")
	}

	return nil
}

type standaloneDiskRelationship struct {
	assigned        bool
	attachmentState *core.VirtualMachineDiskAttachmentStateEnum
}

func readStandaloneDiskRelationship(ctx context.Context, m *Meta, diskID string) (standaloneDiskRelationship, error) {
	var obs standaloneDiskRelationship
	res, err := m.Core.GetDiskWithResponse(ctx, &core.GetDiskParams{DiskId: &diskID})
	if err != nil {
		if res != nil {
			err = genericAPIError(err, res.Body)
		}
		return obs, err
	}
	if res == nil || res.JSON200 == nil {
		return obs, fmt.Errorf("unexpected empty response fetching disk")
	}
	if !res.JSON200.Disk.VirtualMachineDisk.IsSpecified() ||
		res.JSON200.Disk.VirtualMachineDisk.IsNull() {
		return obs, nil
	}
	vmd, err := res.JSON200.Disk.VirtualMachineDisk.Get()
	if err != nil {
		return obs, err
	}
	obs.assigned = true
	obs.attachmentState = vmd.State
	return obs, nil
}

//nolint:lll,goconst // Keep the resize matrix and its actionable diagnostics together.
func effectiveDiskResizeMethod(oldSize, newSize int64, configured string, assigned bool, attachmentState *core.VirtualMachineDiskAttachmentStateEnum) (core.ResizeMethodEnum, error) {
	attached := false
	if assigned {
		if attachmentState == nil {
			return "", fmt.Errorf("assigned disk attachment state is unknown")
		}
		switch *attachmentState {
		case core.VirtualMachineDiskAttachmentStateEnumAttached:
			attached = true
		case core.VirtualMachineDiskAttachmentStateEnumDetached:
		case core.VirtualMachineDiskAttachmentStateEnumAttaching,
			core.VirtualMachineDiskAttachmentStateEnumDetaching,
			core.VirtualMachineDiskAttachmentStateEnumFailed:
			return "", fmt.Errorf("disk attachment state %q is transitional; retry after it settles", *attachmentState)
		default:
			return "", fmt.Errorf("disk attachment state %q is unknown", *attachmentState)
		}
	}
	if newSize < oldSize {
		if attached {
			return "", fmt.Errorf("shrinking requires physical detachment; detach with katapult_disk_assignment or stop the Virtual Machine first")
		}
		return core.Offline, nil
	}
	if configured == "online" && attached {
		return core.Online, nil
	}
	if attached {
		return "", fmt.Errorf("offline growth requires physical detachment; detach with katapult_disk_assignment, stop the Virtual Machine, or explicitly set resize_method = \"online\"")
	}
	return core.Offline, nil
}
