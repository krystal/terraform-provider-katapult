package v6provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/krystal/go-katapult/next/core"
)

type DiskAssignmentResource struct {
	M *Meta
}

type DiskAssignmentResourceModel struct {
	ID               types.String   `tfsdk:"id"`
	VirtualMachineID types.String   `tfsdk:"virtual_machine_id"`
	DiskID           types.String   `tfsdk:"disk_id"`
	Attached         types.Bool     `tfsdk:"attached"`
	AttachOnBoot     types.Bool     `tfsdk:"attach_on_boot"`
	AttachmentState  types.String   `tfsdk:"attachment_state"`
	Timeouts         timeouts.Value `tfsdk:"timeouts"`
}

type diskAssignmentObservation struct {
	vmState         core.VirtualMachineStateEnum
	vmID            string
	assigned        bool
	boot            *bool
	attachOnBoot    *bool
	attachmentState *core.VirtualMachineDiskAttachmentStateEnum
}

const diskAssignmentMarkdownDescription = "Manages the assignment and attachment " +
	"policy of one additional disk on one Virtual Machine. Assignments happen " +
	"after the VM's first boot and never start or stop the VM. A workload that " +
	"needs the disk during first guest boot must arrange a later controlled restart " +
	"or handle that dependency inside the guest.\n\n" +
	"For a running VM, `attached = true` enables attach-on-boot and physically " +
	"attaches the disk. For a stopped VM it enables attach-on-boot while physical " +
	"state may remain detached. `attached = false` disables attach-on-boot and " +
	"physically detaches regardless of VM power. The computed `attach_on_boot` and " +
	"`attachment_state` attributes expose those raw observations.\n\n" +
	"Import an existing relationship with `terraform import " +
	"katapult_disk_assignment.NAME VM_ID/DISK_ID`. Destroy this resource before " +
	"destroying its VM or disk; Terraform references provide graph-safe detach and " +
	"unassign ordering without deleting either endpoint."

func (r *DiskAssignmentResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_disk_assignment"
}

func (r *DiskAssignmentResource) Configure(
	_ context.Context,
	req resource.ConfigureRequest,
	resp *resource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		return
	}
	meta, ok := req.ProviderData.(*Meta)
	if !ok {
		resp.Diagnostics.AddError("Meta Error", "meta is not of type *Meta")
		return
	}
	r.M = meta
}

//nolint:goconst // Terraform attribute and block names are clearer inline in schema declarations.
func (r *DiskAssignmentResource) Schema(
	ctx context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: diskAssignmentMarkdownDescription,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "Synthetic relationship identity in " +
					"`VM_ID/DISK_ID` form.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"virtual_machine_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The Virtual Machine endpoint.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"disk_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The additional disk endpoint.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"attached": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
				MarkdownDescription: "Desired attachment policy. While the VM is " +
					"stopped, true means attach-on-boot is enabled; physical " +
					"attachment occurs when the VM next starts.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"attach_on_boot": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Observed raw attach-on-boot policy.",
			},
			"attachment_state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Observed physical attachment state.",
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

//nolint:lll // Preserve complete operator-facing lifecycle diagnostics.
func (r *DiskAssignmentResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan DiskAssignmentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	vmID, diskID := plan.VirtualMachineID.ValueString(), plan.DiskID.ValueString()
	if vmID == "" || diskID == "" {
		resp.Diagnostics.AddError("Create Error", "virtual_machine_id and disk_id must be known before creation")
		return
	}
	timeout, diags := plan.Timeouts.Create(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	unlock := r.M.lockDiskAssignments(vmID)
	defer unlock()

	obs, err := readDiskAssignmentObservation(ctx, r.M, vmID, diskID)
	if err != nil {
		resp.Diagnostics.AddError("Create Error", err.Error())
		return
	}
	if obs.vmState != core.Started && obs.vmState != core.Stopped {
		resp.Diagnostics.AddError("Create Error", fmt.Sprintf("Virtual Machine %s is in transitional state %s; retry after it settles", vmID, obs.vmState))
		return
	}
	if obs.assigned {
		if obs.vmID != vmID {
			resp.Diagnostics.AddError("Create Error", fmt.Sprintf("disk %s is already assigned to Virtual Machine %s", diskID, obs.vmID))
			return
		}
		resp.Diagnostics.AddError("Disk Assignment Already Exists", fmt.Sprintf(
			"The relationship already exists. Import it with `terraform import katapult_disk_assignment.<name> %s/%s`.", vmID, diskID,
		))
		return
	}

	assignRes, err := r.M.Core.PostDiskAssignWithResponse(ctx,
		core.PostDiskAssignJSONRequestBody{
			Disk:           core.DiskLookup{Id: &diskID},
			VirtualMachine: core.VirtualMachineLookup{Id: &vmID},
		})
	if err != nil {
		if assignRes != nil {
			err = genericAPIError(err, assignRes.Body)
		}
		resp.Diagnostics.AddError("Create Error", err.Error())
		return
	}
	if assignRes == nil || assignRes.JSON200 == nil {
		resp.Diagnostics.AddError("Create Error", "unexpected empty response assigning disk")
		return
	}

	plan.ID = types.StringValue(assignmentID(vmID, diskID))
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err = reconcileDiskAssignment(ctx, r.M, vmID, diskID, plan.Attached.ValueBool(), timeout); err != nil {
		if readErr := r.readIntoModel(ctx, &plan); readErr == nil {
			resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
		}
		resp.Diagnostics.AddError("Create Error", err.Error())
		return
	}
	if err = r.readIntoModel(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Read Error", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *DiskAssignmentResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state DiskAssignmentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readIntoModel(ctx, &state); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read Error", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *DiskAssignmentResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan DiskAssignmentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	vmID, diskID := plan.VirtualMachineID.ValueString(), plan.DiskID.ValueString()
	timeout, diags := plan.Timeouts.Update(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	unlock := r.M.lockDiskAssignments(vmID)
	defer unlock()
	if err := reconcileDiskAssignment(ctx, r.M, vmID, diskID, plan.Attached.ValueBool(), timeout); err != nil {
		if readErr := r.readIntoModel(ctx, &plan); readErr == nil {
			resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
		}
		resp.Diagnostics.AddError("Update Error", err.Error())
		return
	}
	if err := r.readIntoModel(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Read Error", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

//nolint:gocyclo,lll // Safety guards are deliberately kept in lifecycle order.
func (r *DiskAssignmentResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state DiskAssignmentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	vmID, diskID := state.VirtualMachineID.ValueString(), state.DiskID.ValueString()
	timeout, diags := state.Timeouts.Delete(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	unlock := r.M.lockDiskAssignments(vmID)
	defer unlock()

	obs, err := readDiskAssignmentObservation(ctx, r.M, vmID, diskID)
	if errors.Is(err, core.ErrNotFound) || (err == nil && !obs.assigned) {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Delete Error", err.Error())
		return
	}
	if obs.vmID != vmID {
		resp.Diagnostics.AddError("Delete Error", fmt.Sprintf("disk %s is now assigned to a different Virtual Machine %s; refusing to mutate it", diskID, obs.vmID))
		return
	}
	if obs.boot == nil {
		resp.Diagnostics.AddError("Delete Error", "cannot determine whether the disk is the boot disk; refusing to unassign it")
		return
	}
	if *obs.boot {
		resp.Diagnostics.AddError("Delete Error", "refusing to unassign a boot disk")
		return
	}
	if obs.vmState != core.Started && obs.vmState != core.Stopped {
		resp.Diagnostics.AddError("Delete Error", fmt.Sprintf("Virtual Machine %s is in transitional state %s; retry after it settles", vmID, obs.vmState))
		return
	}
	if err = patchDiskAttachOnBoot(ctx, r.M, diskID, false); err != nil {
		resp.Diagnostics.AddError("Delete Error", err.Error())
		return
	}
	if obs.attachmentState != nil && *obs.attachmentState == core.VirtualMachineDiskAttachmentStateEnumAttached {
		if err = detachDiskAndWait(ctx, r.M, diskID, timeout); err != nil {
			detachErr := err
			if errors.Is(err, core.ErrNotFound) {
				obs, err = readDiskAssignmentObservation(ctx, r.M, vmID, diskID)
				if errors.Is(err, core.ErrNotFound) || (err == nil && !obs.assigned) {
					return
				}
			}
			resp.Diagnostics.AddError("Delete Error", detachErr.Error())
			return
		}
	} else if obs.attachmentState == nil || *obs.attachmentState != core.VirtualMachineDiskAttachmentStateEnumDetached {
		resp.Diagnostics.AddError("Delete Error", "disk attachment state is transitional or unknown; retry after it settles")
		return
	}

	unassignRes, err := r.M.Core.PostDiskUnassignWithResponse(ctx,
		core.PostDiskUnassignJSONRequestBody{Disk: core.DiskLookup{Id: &diskID}})
	if err != nil {
		if unassignRes != nil {
			err = genericAPIError(err, unassignRes.Body)
		}
		resp.Diagnostics.AddError("Delete Error", err.Error())
		return
	}
	if unassignRes == nil || unassignRes.JSON200 == nil {
		resp.Diagnostics.AddError("Delete Error", "unexpected empty response unassigning disk")
		return
	}
	if err = waitForDiskAssignmentAbsent(ctx, r.M, vmID, diskID, timeout); err != nil {
		resp.Diagnostics.AddError("Delete Error", err.Error())
	}
}

//nolint:lll // Preserve complete import diagnostics.
func (r *DiskAssignmentResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	vmID, diskID, err := parseAssignmentID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Import Error", err.Error())
		return
	}
	obs, err := readDiskAssignmentObservation(ctx, r.M, vmID, diskID)
	if err != nil {
		resp.Diagnostics.AddError("Import Error", err.Error())
		return
	}
	if !obs.assigned || obs.vmID != vmID {
		resp.Diagnostics.AddError("Import Error", "the requested VM/disk relationship does not exist")
		return
	}
	if obs.boot == nil {
		resp.Diagnostics.AddError("Import Error", "cannot determine whether the disk is the boot disk")
		return
	}
	if *obs.boot {
		resp.Diagnostics.AddError("Import Error", "boot disks are owned by katapult_virtual_machine.system_disk")
		return
	}
	if obs.attachOnBoot == nil || obs.attachmentState == nil ||
		(obs.vmState != core.Started && obs.vmState != core.Stopped) {
		resp.Diagnostics.AddError("Import Error", "the relationship policy or Virtual Machine state is incomplete or transitional")
		return
	}
	attached := *obs.attachOnBoot
	if obs.vmState == core.Started {
		attached = attached && *obs.attachmentState == core.VirtualMachineDiskAttachmentStateEnumAttached
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), assignmentID(vmID, diskID))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("virtual_machine_id"), vmID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("disk_id"), diskID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("attached"), attached)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("attach_on_boot"), *obs.attachOnBoot)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("attachment_state"), string(*obs.attachmentState))...)
}

func (r *DiskAssignmentResource) readIntoModel(ctx context.Context, model *DiskAssignmentResourceModel) error {
	vmID, diskID := model.VirtualMachineID.ValueString(), model.DiskID.ValueString()
	obs, err := readDiskAssignmentObservation(ctx, r.M, vmID, diskID)
	if err != nil {
		return err
	}
	if !obs.assigned || obs.vmID != vmID {
		return core.ErrNotFound
	}
	if obs.boot != nil && *obs.boot {
		return fmt.Errorf("disk %s is the boot disk and cannot be managed as an assignment", diskID)
	}
	if obs.attachOnBoot == nil || obs.attachmentState == nil {
		return fmt.Errorf("disk assignment policy is incomplete or transitional")
	}
	model.ID = types.StringValue(assignmentID(vmID, diskID))
	model.AttachOnBoot = types.BoolValue(*obs.attachOnBoot)
	model.AttachmentState = types.StringValue(string(*obs.attachmentState))
	attached, err := projectDiskAssignmentAttached(model.Attached, obs)
	if err != nil {
		return fmt.Errorf("virtual machine %s: %w", vmID, err)
	}
	model.Attached = attached
	return nil
}

func projectDiskAssignmentAttached(desired types.Bool, obs diskAssignmentObservation) (types.Bool, error) {
	if obs.attachOnBoot == nil || obs.attachmentState == nil {
		return types.BoolNull(), fmt.Errorf("disk assignment policy is incomplete or transitional")
	}

	var observed bool
	switch obs.vmState {
	case core.Started:
		observed = *obs.attachOnBoot &&
			*obs.attachmentState == core.VirtualMachineDiskAttachmentStateEnumAttached
	case core.Stopped:
		observed = *obs.attachOnBoot
	case core.Allocated,
		core.Allocating,
		core.Failed,
		core.Migrating,
		core.Orphaned,
		core.Resetting,
		core.ShuttingDown,
		core.Starting,
		core.Stopping,
		core.Transferring:
		return types.BoolNull(), fmt.Errorf("is in transitional state %s", obs.vmState)
	default:
		return types.BoolNull(), fmt.Errorf("is in unknown state %q", obs.vmState)
	}
	if desired.IsNull() || desired.IsUnknown() {
		return types.BoolValue(observed), nil
	}

	var converged bool
	if desired.ValueBool() {
		converged = *obs.attachOnBoot &&
			(obs.vmState == core.Stopped ||
				*obs.attachmentState == core.VirtualMachineDiskAttachmentStateEnumAttached)
	} else {
		converged = !*obs.attachOnBoot &&
			*obs.attachmentState == core.VirtualMachineDiskAttachmentStateEnumDetached
	}
	if converged {
		return desired, nil
	}
	return types.BoolValue(!desired.ValueBool()), nil
}

func assignmentID(vmID, diskID string) string { return vmID + "/" + diskID }

func parseAssignmentID(id string) (string, string, error) {
	parts := strings.Split(id, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("assignment ID must be exactly VM_ID/DISK_ID")
	}
	return parts[0], parts[1], nil
}

//nolint:lll // Generated API method signatures make the observation boundary long.
func readDiskAssignmentObservation(ctx context.Context, m *Meta, vmID, diskID string) (diskAssignmentObservation, error) {
	obs := diskAssignmentObservation{}
	vmRes, err := m.Core.GetVirtualMachineWithResponse(ctx,
		&core.GetVirtualMachineParams{VirtualMachineId: &vmID})
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return obs, core.ErrNotFound
		}
		if vmRes != nil {
			err = genericAPIError(err, vmRes.Body)
		}
		return obs, err
	}
	if vmRes == nil || vmRes.JSON200 == nil || vmRes.JSON200.VirtualMachine.State == nil {
		return obs, fmt.Errorf("unexpected incomplete response fetching Virtual Machine %s", vmID)
	}
	obs.vmState = *vmRes.JSON200.VirtualMachine.State

	diskRes, err := m.Core.GetDiskWithResponse(ctx, &core.GetDiskParams{DiskId: &diskID})
	if err != nil {
		if errors.Is(err, core.ErrNotFound) || (diskRes != nil && isErrNotFoundOrInTrash(err, diskRes.JSON406)) {
			return obs, core.ErrNotFound
		}
		if diskRes != nil {
			err = genericAPIError(err, diskRes.Body)
		}
		return obs, err
	}
	if diskRes == nil || diskRes.JSON200 == nil {
		return obs, fmt.Errorf("unexpected empty response fetching disk %s", diskID)
	}
	if !diskRes.JSON200.Disk.VirtualMachineDisk.IsSpecified() ||
		diskRes.JSON200.Disk.VirtualMachineDisk.IsNull() {
		return obs, nil
	}
	vmd, err := diskRes.JSON200.Disk.VirtualMachineDisk.Get()
	if err != nil {
		return obs, fmt.Errorf("reading disk relationship: %w", err)
	}
	obs.assigned = true
	obs.boot = vmd.Boot
	obs.attachOnBoot = vmd.AttachOnBoot
	obs.attachmentState = vmd.State
	if vmd.VirtualMachine != nil && vmd.VirtualMachine.Id != nil {
		obs.vmID = *vmd.VirtualMachine.Id
	}
	return obs, nil
}

//nolint:gocyclo,lll // The explicit state matrix keeps mutation preconditions auditable.
func reconcileDiskAssignment(ctx context.Context, m *Meta, vmID, diskID string, attached bool, timeout time.Duration) error {
	obs, err := readDiskAssignmentObservation(ctx, m, vmID, diskID)
	if err != nil {
		return err
	}
	if !obs.assigned || obs.vmID != vmID {
		return fmt.Errorf("disk %s is not assigned to Virtual Machine %s", diskID, vmID)
	}
	if obs.boot == nil {
		return fmt.Errorf("cannot determine whether disk %s is the boot disk", diskID)
	}
	if *obs.boot {
		return fmt.Errorf("disk %s is the boot disk and cannot be managed as an assignment", diskID)
	}
	if obs.attachmentState == nil {
		return fmt.Errorf("disk %s attachment state is unknown", diskID)
	}
	if obs.vmState != core.Started && obs.vmState != core.Stopped {
		return fmt.Errorf("virtual machine %s is in transitional state %s", vmID, obs.vmState)
	}
	if *obs.attachmentState == core.VirtualMachineDiskAttachmentStateEnumAttaching ||
		*obs.attachmentState == core.VirtualMachineDiskAttachmentStateEnumDetaching {
		return fmt.Errorf("disk %s attachment is transitional; retry after it settles", diskID)
	}
	if err = patchDiskAttachOnBoot(ctx, m, diskID, attached); err != nil {
		return err
	}
	if attached && obs.vmState == core.Started && *obs.attachmentState == core.VirtualMachineDiskAttachmentStateEnumDetached {
		if err = attachDiskAndWait(ctx, m, diskID, timeout); err != nil {
			return err
		}
	}
	if !attached && *obs.attachmentState == core.VirtualMachineDiskAttachmentStateEnumAttached {
		if err = detachDiskAndWait(ctx, m, diskID, timeout); err != nil {
			return err
		}
	}
	verified, err := readDiskAssignmentObservation(ctx, m, vmID, diskID)
	if err != nil {
		return err
	}
	if !verified.assigned || verified.attachOnBoot == nil || *verified.attachOnBoot != attached {
		return fmt.Errorf("disk assignment policy did not converge")
	}
	if attached && verified.vmState == core.Started &&
		(verified.attachmentState == nil || *verified.attachmentState != core.VirtualMachineDiskAttachmentStateEnumAttached) {
		return fmt.Errorf("disk did not become physically attached")
	}
	if !attached && (verified.attachmentState == nil || *verified.attachmentState != core.VirtualMachineDiskAttachmentStateEnumDetached) {
		return fmt.Errorf("disk did not become physically detached")
	}
	return nil
}

func patchDiskAttachOnBoot(ctx context.Context, m *Meta, diskID string, value bool) error {
	res, err := m.Core.PatchDiskWithResponse(ctx, core.PatchDiskJSONRequestBody{
		Disk:       core.DiskLookup{Id: &diskID},
		Properties: core.DiskArguments{VirtualMachineDisk: &core.VirtualMachineDiskArguments{AttachOnBoot: &value}},
	})
	if err != nil {
		if res != nil {
			return genericAPIError(err, res.Body)
		}
		return err
	}
	if res == nil || res.JSON200 == nil {
		return fmt.Errorf("unexpected empty response updating attach-on-boot for disk %s", diskID)
	}
	return nil
}

func attachDiskAndWait(ctx context.Context, m *Meta, diskID string, timeout time.Duration) error {
	res, err := m.Core.PostDiskAttachWithResponse(ctx,
		core.PostDiskAttachJSONRequestBody{Disk: core.DiskLookup{Id: &diskID}})
	if err != nil {
		if res != nil {
			return genericAPIError(err, res.Body)
		}
		return err
	}
	if res == nil || res.JSON200 == nil || res.JSON200.Task.Id == nil {
		return fmt.Errorf("unexpected empty response attaching disk %s", diskID)
	}
	return waitForTaskCompletion(ctx, m, timeout, *res.JSON200.Task.Id)
}

func detachDiskAndWait(ctx context.Context, m *Meta, diskID string, timeout time.Duration) error {
	res, err := m.Core.PostDiskDetachWithResponse(ctx,
		core.PostDiskDetachJSONRequestBody{Disk: core.DiskLookup{Id: &diskID}})
	if err != nil {
		if res != nil && res.JSON422 != nil {
			apiErr := parseGenericAPIError(res.Body)
			if apiErr != nil && apiErr.Code == string(core.UnassignedDisk) {
				return core.ErrNotFound
			}
		}
		if res != nil {
			return genericAPIError(err, res.Body)
		}
		return err
	}
	if res == nil || res.JSON200 == nil || res.JSON200.Task.Id == nil {
		return fmt.Errorf("unexpected empty response detaching disk %s", diskID)
	}
	return waitForTaskCompletion(ctx, m, timeout, *res.JSON200.Task.Id)
}

func waitForDiskAssignmentAbsent(
	ctx context.Context,
	m *Meta,
	vmID, diskID string,
	timeout time.Duration,
) error {
	waiter := &retry.StateChangeConf{
		Pending:      []string{"present"},
		Target:       []string{"absent"},
		Timeout:      timeout,
		Delay:        m.stateChangeDelay(time.Second),
		MinTimeout:   m.stateChangeDelay(2 * time.Second),
		PollInterval: m.stateChangePollInterval(),
		Refresh: func() (interface{}, string, error) {
			obs, err := readDiskAssignmentObservation(ctx, m, vmID, diskID)
			if errors.Is(err, core.ErrNotFound) || (err == nil && !obs.assigned) {
				return obs, "absent", nil
			}
			if err != nil {
				return nil, "", err
			}
			if obs.vmID != vmID {
				return nil, "", fmt.Errorf("disk %s became assigned to different Virtual Machine %s", diskID, obs.vmID)
			}
			return obs, "present", nil
		},
	}
	_, err := waiter.WaitForStateContext(ctx)
	if err != nil {
		return fmt.Errorf("waiting for disk assignment %s to disappear: %w", assignmentID(vmID, diskID), err)
	}
	return nil
}

var _ resource.ResourceWithImportState = (*DiskAssignmentResource)(nil)
