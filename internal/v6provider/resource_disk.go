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

const (
	diskImportInitialFileSystemPrivateKey = "disk_import_initial_file_system_v1"
	diskResizeMethodPrivateKey            = "disk_resize_method_v1"
)

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
	"detach first, then change `size_in_gb`. Disk size increases typically complete " +
	"quickly. Offline growth also expands any supported filesystem, so the additional " +
	"capacity is available when the disk is attached again. Online growth leaves guest " +
	"partition and filesystem expansion to the operator. Offline shrink can take " +
	"substantially longer because Katapult must shrink the filesystem and partition " +
	"before reducing the disk. Shrink requires a recognized partition table and " +
	"shrinkable filesystem. The default update timeout is 2 hours; consider increasing " +
	"it for large shrink operations. Reaching the timeout stops Terraform waiting but " +
	"does not cancel the Katapult resize task, so check its state before retrying. Set " +
	"`initial_file_system = \"ext4\"` for a new disk that Terraform must be able to " +
	"shrink; XFS cannot be shrunk."

type (
	DiskResource struct {
		M *Meta
	}

	DiskResourceModel struct {
		ID                types.String   `tfsdk:"id"`
		Name              types.String   `tfsdk:"name"`
		SizeInGB          types.Int64    `tfsdk:"size_in_gb"`
		InitialFileSystem types.String   `tfsdk:"initial_file_system"`
		StorageSpeed      types.String   `tfsdk:"storage_speed"`
		BusType           types.String   `tfsdk:"bus_type"`
		IOProfileID       types.String   `tfsdk:"io_profile_id"`
		ResizeMethod      types.String   `tfsdk:"resize_method"`
		WWN               types.String   `tfsdk:"wwn"`
		State             types.String   `tfsdk:"state"`
		Timeouts          timeouts.Value `tfsdk:"timeouts"`
	}
)

var _ resource.ResourceWithModifyPlan = (*DiskResource)(nil)

type requiresReplaceAfterImportAdoptionModifier struct{}

func (requiresReplaceAfterImportAdoptionModifier) Description(context.Context) string {
	return "Requires replacement after a previously managed value changes, " +
		"while allowing imported null state to adopt configuration once."
}

func (m requiresReplaceAfterImportAdoptionModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (requiresReplaceAfterImportAdoptionModifier) PlanModifyString(
	ctx context.Context,
	req planmodifier.StringRequest,
	resp *planmodifier.StringResponse,
) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() || req.PlanValue.IsUnknown() ||
		req.StateValue.IsUnknown() || req.PlanValue.Equal(req.StateValue) {
		return
	}
	if req.StateValue.IsNull() {
		eligible := false
		if req.Private != nil {
			value, diags := req.Private.GetKey(ctx, diskImportInitialFileSystemPrivateKey)
			resp.Diagnostics.Append(diags...)
			eligible = len(value) > 0
		}
		if eligible {
			if resp.Private != nil {
				resp.Diagnostics.Append(resp.Private.SetKey(
					ctx, diskImportInitialFileSystemPrivateKey, nil,
				)...)
			}
			return
		}
	}
	resp.RequiresReplace = true
}

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
	if resp.Diagnostics.HasError() || len(resp.RequiresReplace) > 0 ||
		plan.SizeInGB.IsUnknown() || plan.ResizeMethod.IsUnknown() ||
		state.SizeInGB.IsNull() || plan.SizeInGB.Equal(state.SizeInGB) {
		return
	}
	obs, err := readStandaloneDiskRelationship(ctx, r.M, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return
		}
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
			"initial_file_system": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "File system used to initialize the disk: " +
					"`ext4` or `xfs`. When omitted, Katapult creates a blank disk. " +
					"Imported disks do not expose their existing file-system type, so " +
					"the first configured value is adopted into Terraform state without " +
					"recreating the disk; verify that it matches the real disk first. " +
					"Setting a file system later on a blank disk created by this resource, " +
					"or changing an adopted or creation-time value, replaces the disk. Use " +
					"`ext4` when the disk must support offline shrink; XFS cannot be shrunk.",
				Validators: []validator.String{
					stringvalidator.OneOf("ext4", "xfs"),
				},
				PlanModifiers: []planmodifier.String{
					requiresReplaceAfterImportAdoptionModifier{},
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
			},
		},
		Blocks: map[string]schema.Block{
			"timeouts": timeouts.Block(ctx, timeouts.Opts{
				Create: true,
				Update: true,
				UpdateDescription: "Maximum time Terraform waits for a disk update " +
					"to complete. Defaults to 2 hours. Disk growth normally completes " +
					"quickly, including filesystem-aware offline growth. Offline shrink " +
					"may take substantially longer; consider 4 hours or more for large " +
					"disks. Reaching the timeout stops Terraform waiting but does not " +
					"cancel the Katapult resize task, so check its state before retrying.",
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
	if !plan.InitialFileSystem.IsNull() && !plan.InitialFileSystem.IsUnknown() {
		fs := core.FileSystemEnum(plan.InitialFileSystem.ValueString())
		args.InitialFileSystem = &fs
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

	diskID := *createRes.JSON201.Disk.Id

	// Persist the disk ID as soon as the API returns it so state is preserved
	// even when the accompanying task response is incomplete.
	plan.ID = types.StringValue(diskID)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if createRes.JSON201.Task.Id == nil {
		resp.Diagnostics.AddError("Create Error",
			"unexpected response creating disk: missing task ID")
		return
	}

	taskID := *createRes.JSON201.Task.Id

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

	if err := r.patchDiskProperties(ctx, diskID, &plan, &state, timeout); err != nil {
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

// patchDiskProperties applies mutable disk properties through the API operation
// appropriate to each property. No-op when none have changed.
func (r *DiskResource) patchDiskProperties(
	ctx context.Context,
	diskID string,
	plan, state *DiskResourceModel,
	timeout time.Duration,
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
	if patchArgs.Name != nil || patchArgs.BusType != nil {
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
	}

	if !plan.IOProfileID.Equal(state.IOProfileID) && !plan.IOProfileID.IsNull() {
		if err := r.updateDiskIOProfile(
			ctx, diskID, plan.IOProfileID.ValueString(), timeout,
		); err != nil {
			return err
		}
	}

	return nil
}

func (r *DiskResource) updateDiskIOProfile(
	ctx context.Context,
	diskID, profileID string,
	timeout time.Duration,
) error {
	profileRes, err := r.M.Core.PutDiskIoProfileWithResponse(
		ctx,
		core.PutDiskIoProfileJSONRequestBody{
			Disk:      core.DiskLookup{Id: &diskID},
			IoProfile: core.DiskIOProfileLookup{Id: &profileID},
		},
	)
	if err != nil {
		if profileRes != nil {
			return genericAPIError(err, profileRes.Body)
		}
		return err
	}
	if profileRes == nil || profileRes.JSON200 == nil ||
		profileRes.JSON200.Task.Id == nil {
		return fmt.Errorf("unexpected empty response updating disk I/O profile")
	}
	if err := waitForTaskCompletion(
		ctx, r.M, timeout, *profileRes.JSON200.Task.Id,
	); err != nil {
		return fmt.Errorf("updating disk I/O profile: %w", err)
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
	if resizeRes == nil || resizeRes.JSON200 == nil || resizeRes.JSON200.Task.Id == nil {
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
	if diskRes == nil || diskRes.JSON200 == nil {
		resp.Diagnostics.AddError("Delete Error", "cannot verify disk assignment state from an incomplete API response; refusing deletion")
		return
	}
	disk := diskRes.JSON200.Disk
	if !disk.VirtualMachineDisk.IsSpecified() {
		resp.Diagnostics.AddError("Delete Error", "cannot verify disk assignment state because virtual_machine_disk was omitted; refusing deletion")
		return
	}
	if !disk.VirtualMachineDisk.IsNull() {
		resp.Diagnostics.AddError("Delete Error", fmt.Sprintf(
			"disk %s still has a Virtual Machine assignment; remove the corresponding katapult_disk_assignment first",
			diskID,
		))
		return
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
	resp.Diagnostics.Append(resp.Private.SetKey(
		ctx, diskImportInitialFileSystemPrivateKey, []byte("true"),
	)...)
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
	if res == nil || res.JSON200 == nil {
		return fmt.Errorf("unexpected empty response fetching disk")
	}

	disk := res.JSON200.Disk

	if disk.Name != nil {
		model.Name = types.StringValue(*disk.Name)
	}
	if disk.SizeInGb != nil {
		model.SizeInGB = types.Int64Value(int64(*disk.SizeInGb))
	}
	model.StorageSpeed = types.StringNull()
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
	model.WWN = types.StringNull()
	if disk.Wwn != nil {
		model.WWN = types.StringValue(*disk.Wwn)
	}
	model.State = types.StringNull()
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
		if errors.Is(err, core.ErrNotFound) ||
			(res != nil && isErrNotFoundOrInTrash(err, res.JSON406)) {
			return obs, core.ErrNotFound
		}
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
			core.VirtualMachineDiskAttachmentStateEnumDetaching:
			return "", fmt.Errorf("disk attachment state %q is transitional; retry after it settles", *attachmentState)
		case core.VirtualMachineDiskAttachmentStateEnumFailed:
			return "", fmt.Errorf("disk attachment state is failed; repair or remove the failed assignment before resizing")
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
