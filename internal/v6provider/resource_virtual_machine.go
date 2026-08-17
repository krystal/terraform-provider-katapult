package v6provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/jimeh/rands"
	"github.com/krystal/go-katapult/buildspec"
	"github.com/krystal/go-katapult/next/core"
)

type (
	VirtualMachineResource struct {
		M *Meta
	}

	VirtualMachineResourceModel struct {
		ID                  types.String   `tfsdk:"id"`
		Name                types.String   `tfsdk:"name"`
		Hostname            types.String   `tfsdk:"hostname"`
		Description         types.String   `tfsdk:"description"`
		FQDN                types.String   `tfsdk:"fqdn"`
		State               types.String   `tfsdk:"state"`
		PoweredOn           types.Bool     `tfsdk:"powered_on"`
		Package             types.String   `tfsdk:"package"`
		DiskTemplate        types.String   `tfsdk:"disk_template"`
		DiskTemplateOptions types.Map      `tfsdk:"disk_template_options"`
		Disk                types.List     `tfsdk:"disk"`
		SystemDisk          types.Object   `tfsdk:"system_disk"`
		IPAddressIDs        types.Set      `tfsdk:"ip_address_ids"`
		IPAddresses         types.Set      `tfsdk:"ip_addresses"`
		VirtualNetworkIDs   types.Set      `tfsdk:"virtual_network_ids"`
		NetworkSpeedProfile types.String   `tfsdk:"network_speed_profile"`
		NetworkInterfaces   types.List     `tfsdk:"network_interfaces"`
		Tags                types.Set      `tfsdk:"tags"`
		GroupID             types.String   `tfsdk:"group_id"`
		Timeouts            timeouts.Value `tfsdk:"timeouts"`
	}

	VirtualMachineDiskModel struct {
		Name types.String `tfsdk:"name"`
		Size types.Int64  `tfsdk:"size"`
	}
)

type virtualMachinePackageReader interface {
	GetVirtualMachineWithResponse(
		context.Context,
		*core.GetVirtualMachineParams,
		...core.RequestEditorFn,
	) (*core.GetVirtualMachineResponse, error)
	GetVirtualMachinePackageWithResponse(
		context.Context,
		*core.GetVirtualMachinePackageParams,
		...core.RequestEditorFn,
	) (*core.GetVirtualMachinePackageResponse, error)
}

const (
	virtualMachineDiskSizeAttribute               = "size"
	virtualMachineShutdownActionLabel             = "shutdown"
	virtualMachineImportDiskTemplatePrivateKey    = "virtual_machine_import_disk_template_v1"
	virtualMachineImportTemplateOptionsPrivateKey = "virtual_machine_import_disk_template_options_v1"
	virtualMachineLegacyDiskIDsPrivateKey         = "virtual_machine_legacy_disk_ids_v1"
	virtualMachineSystemDiskResizePrivateKey      = "virtual_machine_system_disk_resize_method_v1"
)

const virtualMachineMarkdownDescription = "Manages a Virtual Machine in Katapult.\n\n" +
	"~> **Warning:** Deleting a virtual machine resource will by default purge " +
	"the VM from Katapult's trash, permanently deleting it. Set " +
	"`skip_trash_object_purge` on the provider to keep it in the trash instead.\n\n" +
	"Set `powered_on` explicitly to opt into ongoing power-state management. " +
	"Omitting it leaves power state unmanaged after creation. A VM created with " +
	"`powered_on = false` is initially started by Katapult's build process and " +
	"then gracefully shut down before creation completes, so connection-based " +
	"provisioners cannot run against the stopped result.\n\n" +
	"The VM owns only its boot disk through `system_disk`. Additional disks are " +
	"independent `katapult_disk` objects whose relationships are owned by " +
	"`katapult_disk_assignment` after the VM's first boot. VM deletion refuses " +
	"remaining non-boot relationships, and disk deletion refuses every remaining " +
	"relationship; remove assignment resources first so Terraform's dependency " +
	"graph performs detach and unassign before endpoint deletion. System disk growth " +
	"typically completes quickly, including filesystem-aware offline growth. Offline " +
	"shrink can take substantially longer because Katapult must shrink the filesystem " +
	"and partition before reducing the disk. The default VM update timeout is 10 " +
	"minutes; increase `timeouts.update` for large system disk shrink operations. " +
	"Reaching the timeout stops Terraform waiting but does not cancel the Katapult " +
	"resize task, so check its state before retrying.\n\n" +
	"To migrate deprecated `disk` blocks, first refresh and use " +
	"`katapult_virtual_machine_disks` to inventory assignments. Keep all legacy " +
	"blocks while declaring and importing each additional disk and its " +
	"`VM_ID/DISK_ID` assignment. Then remove every legacy block and optionally " +
	"add an equivalent `system_disk` object for the former first entry. Terraform " +
	"providers cannot inspect whether sibling resources were imported, so skipping " +
	"the first stage leaves additional disks unmanaged even though the VM migration " +
	"itself is non-destructive. VMs created by older provider versions do not have " +
	"the exact deprecated disk identities needed for safe cascade deletion, so a VM " +
	"that still has additional legacy relationships must complete this migration " +
	"before Terraform can destroy it.\n\n" +
	"Importing a VM discovers its boot disk but does not create sibling resources. " +
	"Import the VM by VM ID, then import every additional disk by disk ID and " +
	"every relationship by `VM_ID/DISK_ID`. Disk-template identity and options " +
	"configured after import are adopted once when they match the discovered " +
	"installation rather than replacing the VM."

var vmNetworkInterfaceAttrTypes = map[string]attr.Type{
	"id":                 types.StringType,
	"network_id":         types.StringType,
	"virtual_network_id": types.StringType,
	"mac_address":        types.StringType,
	"ip_addresses":       types.SetType{ElemType: types.StringType},
}

var _ resource.ResourceWithModifyPlan = (*VirtualMachineResource)(nil)

// vmGroupPatchBody is a custom PATCH body that allows explicitly sending
// "group": null to clear the VM group, which the SDK struct cannot express
// due to its omitempty tag.
type vmGroupPatchBody struct {
	VirtualMachine core.VirtualMachineLookup `json:"virtual_machine"`
	Properties     vmGroupPatchProperties    `json:"properties"`
}

// vmGroupPatchProperties embeds VirtualMachineArguments and shadows the
// Group field with a *json.RawMessage so null can be marshaled explicitly.
type vmGroupPatchProperties struct {
	core.VirtualMachineArguments
	Group *json.RawMessage `json:"group,omitempty"`
}

func (r *VirtualMachineResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_virtual_machine"
}

func (r *VirtualMachineResource) Configure(
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

//nolint:funlen,gocyclo,lll // Plan-time migration and resize guards intentionally remain in evaluation order.
func (r *VirtualMachineResource) ModifyPlan(
	ctx context.Context,
	req resource.ModifyPlanRequest,
	resp *resource.ModifyPlanResponse,
) {
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan VirtualMachineResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var configuredLegacy types.List
	var configuredSystem types.Object
	var configuredDiskTemplate types.String
	var configuredDiskTemplateOptions types.Map
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("disk"), &configuredLegacy)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("system_disk"), &configuredSystem)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("disk_template"), &configuredDiskTemplate)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("disk_template_options"), &configuredDiskTemplateOptions)...)
	if resp.Diagnostics.HasError() {
		return
	}
	legacyConfigured := !configuredLegacy.IsNull() && !configuredLegacy.IsUnknown() && len(configuredLegacy.Elements()) > 0
	systemConfigured := !configuredSystem.IsNull() && !configuredSystem.IsUnknown()
	if legacyConfigured && systemConfigured {
		resp.Diagnostics.AddError("Conflicting Disk Configuration", "Configure either deprecated disk blocks or system_disk, not both.")
		return
	}
	if req.State.Raw.IsNull() {
		if err := validateChangedLegacyDiskSizes(ctx, types.List{}, plan.Disk); err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("disk"), "Invalid Legacy Disk Size", err.Error())
		}
		return
	}
	if r.M == nil {
		return
	}

	var state VirtualMachineResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	priorSystem, priorSystemDiags := decodeVirtualMachineSystemDisk(ctx, state.SystemDisk)
	resp.Diagnostics.Append(priorSystemDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	templateImportEligible := false
	templateOptionsImportEligible := false
	if req.Private != nil {
		value, diags := req.Private.GetKey(ctx, virtualMachineImportDiskTemplatePrivateKey)
		resp.Diagnostics.Append(diags...)
		templateImportEligible = len(value) > 0
		value, diags = req.Private.GetKey(ctx, virtualMachineImportTemplateOptionsPrivateKey)
		resp.Diagnostics.Append(diags...)
		templateOptionsImportEligible = len(value) > 0
		if resp.Diagnostics.HasError() {
			return
		}
	}
	priorLegacy := !state.Disk.IsNull() && !state.Disk.IsUnknown() && len(state.Disk.Elements()) > 0
	plannedLegacy := !plan.Disk.IsNull() && !plan.Disk.IsUnknown() && len(plan.Disk.Elements()) > 0
	if err := validateChangedLegacyDiskSizes(ctx, state.Disk, plan.Disk); err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("disk"), "Invalid Legacy Disk Size", err.Error())
		return
	}
	switch {
	case priorLegacy && plannedLegacy && !plan.Disk.Equal(state.Disk):
		resp.RequiresReplace = append(resp.RequiresReplace, path.Root("disk"))
	case !priorLegacy && plannedLegacy && legacyConfigured &&
		(templateImportEligible || templateOptionsImportEligible):
		resp.Diagnostics.AddAttributeError(
			path.Root("disk"),
			"Imported Virtual Machine Legacy Disk Configuration",
			"Deprecated disk blocks cannot safely adopt an imported Virtual Machine's existing disks. Leave disk unset, manage the existing boot disk through system_disk, import each additional disk into katapult_disk, and import each VM/disk relationship into katapult_disk_assignment.",
		)
		return
	case !priorLegacy && plannedLegacy:
		resp.RequiresReplace = append(resp.RequiresReplace, path.Root("disk"))
	case priorLegacy && !plannedLegacy:
		additionalIDs, err := validateLegacyDiskMigration(
			ctx, r.M, state.ID.ValueString(), priorSystem, plan.SystemDisk, systemConfigured,
		)
		if err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("disk"), "Unsafe Legacy Disk Migration", err.Error())
			return
		}
		additionalRelationships := "none"
		if len(additionalIDs) > 0 {
			relationships := make([]string, 0, len(additionalIDs))
			for _, diskID := range additionalIDs {
				relationships = append(relationships, assignmentID(state.ID.ValueString(), diskID))
			}
			additionalRelationships = strings.Join(relationships, ", ")
		}
		resp.Diagnostics.AddWarning("Legacy Disk Schema Migration", fmt.Sprintf(
			"The deprecated disk blocks are being removed without changing remote disks. Observed additional relationships: %s. Import each additional disk into katapult_disk and each relationship into katapult_disk_assignment before applying if Terraform should continue to own them.",
			additionalRelationships,
		))
	}

	if templateImportEligible && !configuredDiskTemplate.IsNull() && !configuredDiskTemplate.IsUnknown() {
		configuredRef := configuredDiskTemplate.ValueString()
		adopt := state.DiskTemplate.IsNull() || state.DiskTemplate.IsUnknown() ||
			virtualMachineDiskTemplateRefsEquivalent(configuredRef, state.DiskTemplate.ValueString())
		if !adopt {
			refs, err := fetchDiskTemplateReferences(ctx, r.M, priorSystem.ID.ValueString())
			if err != nil {
				resp.Diagnostics.AddAttributeError(path.Root("disk_template"), "Cannot Validate Imported Disk Template", err.Error())
				return
			}
			for _, ref := range refs {
				if virtualMachineDiskTemplateRefsEquivalent(configuredRef, ref) {
					adopt = true
					break
				}
			}
		}
		if adopt {
			resp.Diagnostics.Append(resp.Private.SetKey(ctx, virtualMachineImportDiskTemplatePrivateKey, nil)...)
		} else if !plan.DiskTemplate.Equal(state.DiskTemplate) {
			resp.RequiresReplace = append(resp.RequiresReplace, path.Root("disk_template"))
		}
	} else if !state.DiskTemplate.IsNull() && !state.DiskTemplate.IsUnknown() &&
		!plan.DiskTemplate.IsNull() && !plan.DiskTemplate.IsUnknown() &&
		!plan.DiskTemplate.Equal(state.DiskTemplate) {
		resp.RequiresReplace = append(resp.RequiresReplace, path.Root("disk_template"))
	}
	if templateOptionsImportEligible && !configuredDiskTemplateOptions.IsNull() && !configuredDiskTemplateOptions.IsUnknown() {
		resp.Diagnostics.Append(resp.Private.SetKey(ctx, virtualMachineImportTemplateOptionsPrivateKey, nil)...)
	} else if !state.DiskTemplateOptions.IsNull() && !state.DiskTemplateOptions.IsUnknown() &&
		!plan.DiskTemplateOptions.Equal(state.DiskTemplateOptions) {
		resp.RequiresReplace = append(resp.RequiresReplace, path.Root("disk_template_options"))
	}
	if resp.Diagnostics.HasError() {
		return
	}

	if state.ID.IsNull() || state.ID.IsUnknown() {
		return
	}
	var poweredOn types.Bool
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("powered_on"), &poweredOn)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !plan.Package.IsNull() && !plan.Package.IsUnknown() && !plan.Package.Equal(state.Package) {
		err := validateVirtualMachinePackageChange(ctx, r.M.Core, state.ID.ValueString(), plan.Package.ValueString(), poweredOn)
		if err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("package"), "Invalid Virtual Machine Package Change", err.Error())
		}
	}

	plannedSystem, plannedSystemDiags := decodeVirtualMachineSystemDisk(ctx, plan.SystemDisk)
	resp.Diagnostics.Append(plannedSystemDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resizingSystemDisk := !plan.SystemDisk.IsNull() && !plan.SystemDisk.IsUnknown() &&
		!state.SystemDisk.IsNull() && !state.SystemDisk.IsUnknown() &&
		!plannedSystem.SizeInGB.IsUnknown() && !plannedSystem.SizeInGB.Equal(priorSystem.SizeInGB)
	if !resizingSystemDisk {
		if resp.Private != nil {
			resp.Diagnostics.Append(resp.Private.SetKey(ctx, virtualMachineSystemDiskResizePrivateKey, nil)...)
		}
		return
	}

	vmState, err := fetchVirtualMachineState(ctx, r.M, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("system_disk").AtName("size_in_gb"), "Cannot Validate System Disk Resize", err.Error())
		return
	}
	if vmState != core.Started && vmState != core.Stopped {
		resp.Diagnostics.AddAttributeError(path.Root("system_disk").AtName("size_in_gb"), "Cannot Validate System Disk Resize", fmt.Sprintf("Virtual Machine is in transitional state %s; retry after it settles", vmState))
		return
	}
	resizeState := vmState
	if !poweredOn.IsNull() && !poweredOn.IsUnknown() && !poweredOn.ValueBool() {
		resizeState = core.Stopped
	}
	attachmentState := core.VirtualMachineDiskAttachmentStateEnumDetached
	if resizeState == core.Started {
		attachmentState = core.VirtualMachineDiskAttachmentStateEnumAttached
	}
	method, err := effectiveDiskResizeMethod(
		priorSystem.SizeInGB.ValueInt64(), plannedSystem.SizeInGB.ValueInt64(),
		plannedSystem.ResizeMethod.ValueString(), true, &attachmentState,
	)
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("system_disk").AtName("size_in_gb"), "Invalid System Disk Resize", err.Error())
		return
	}
	if method == core.Online {
		resp.Diagnostics.AddAttributeWarning(path.Root("system_disk").AtName("size_in_gb"), "Online System Disk Growth", "Katapult grows the attached block device only; expand the guest partition and filesystem manually.")
	}
	if plannedSystem.SizeInGB.ValueInt64() < priorSystem.SizeInGB.ValueInt64() {
		resp.Diagnostics.AddAttributeWarning(path.Root("system_disk").AtName("size_in_gb"), "Offline System Disk Shrink", "Katapult must shrink the filesystem and partition before the disk; insufficient capacity fails the task, and large disks may need a longer update timeout.")
	}
	encodedMethod, err := json.Marshal(string(method))
	if err != nil {
		resp.Diagnostics.AddError("System Disk Resize Plan Error", err.Error())
		return
	}
	if resp.Private != nil {
		resp.Diagnostics.Append(resp.Private.SetKey(ctx, virtualMachineSystemDiskResizePrivateKey, encodedMethod)...)
	}
}

func validateChangedLegacyDiskSizes(ctx context.Context, state, plan types.List) error {
	if plan.IsNull() || plan.IsUnknown() || plan.Equal(state) {
		return nil
	}

	var plannedDisks []VirtualMachineDiskModel
	diags := plan.ElementsAs(ctx, &plannedDisks, false)
	if diags.HasError() {
		return fmt.Errorf("reading planned deprecated disk blocks: %s", diags)
	}
	for _, planned := range plannedDisks {
		if !planned.Size.IsNull() && !planned.Size.IsUnknown() && planned.Size.ValueInt64() < 10 {
			return fmt.Errorf(
				"changed deprecated disk configurations require every disk to be at least 10 GB; got %d GB",
				planned.Size.ValueInt64(),
			)
		}
	}
	return nil
}

//nolint:gocyclo,lll // Migration validation is an explicit, fail-closed sequence.
func validateLegacyDiskMigration(
	ctx context.Context,
	m *Meta,
	vmID string,
	priorSystem VirtualMachineSystemDiskModel,
	plannedSystemValue types.Object,
	systemConfigured bool,
) ([]string, error) {
	attachments, err := fetchAllVMDisks(ctx, m, vmID)
	if err != nil {
		return nil, fmt.Errorf("refreshing VM disk relationships: %w", err)
	}
	if _, err := virtualMachineDiskAttachmentIDs(attachments); err != nil {
		return nil, fmt.Errorf("cannot inventory VM disk relationships: %w", err)
	}
	boot, authoritative := selectBootDiskAssignment(attachments, priorSystem.ID.ValueString())
	if !authoritative || boot == nil || boot.Disk == nil || boot.Disk.Id == nil {
		return nil, fmt.Errorf("cannot identify one authoritative boot disk; refresh state and resolve ambiguous boot relationships before removing deprecated disk blocks")
	}
	bootID := *boot.Disk.Id
	if priorSystem.ID.ValueString() != "" && priorSystem.ID.ValueString() != bootID {
		return nil, fmt.Errorf("refreshed boot disk %s does not match prior system_disk identity %s", bootID, priorSystem.ID.ValueString())
	}

	if systemConfigured {
		plannedSystem, diags := decodeVirtualMachineSystemDisk(ctx, plannedSystemValue)
		if diags.HasError() {
			return nil, fmt.Errorf("reading planned system_disk: %s", diags)
		}
		if !plannedSystem.Name.IsNull() && !plannedSystem.Name.IsUnknown() &&
			!priorSystem.Name.IsNull() && !priorSystem.Name.IsUnknown() &&
			!plannedSystem.Name.Equal(priorSystem.Name) {
			return nil, fmt.Errorf("planned system_disk.name differs from the refreshed boot disk; first migrate with equivalent values, then rename in a later apply")
		}
		if !plannedSystem.SizeInGB.IsNull() && !plannedSystem.SizeInGB.IsUnknown() &&
			!priorSystem.SizeInGB.IsNull() && !priorSystem.SizeInGB.IsUnknown() &&
			!plannedSystem.SizeInGB.Equal(priorSystem.SizeInGB) {
			return nil, fmt.Errorf("planned system_disk.size_in_gb differs from the refreshed boot disk; first migrate with equivalent values, then resize in a later apply")
		}
	}

	additionalIDs := make([]string, 0, len(attachments)-1)
	for i := range attachments {
		attachment := &attachments[i]
		if attachment.Disk == nil || attachment.Disk.Id == nil || *attachment.Disk.Id == bootID {
			continue
		}
		additionalIDs = append(additionalIDs, *attachment.Disk.Id)
	}
	sort.Strings(additionalIDs)
	return additionalIDs, nil
}

//nolint:lll // Keep complete schema guidance adjacent to the relevant fields.
func (r *VirtualMachineResource) Schema( //nolint:funlen
	ctx context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: virtualMachineMarkdownDescription,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "The unique identifier of the " +
					"Virtual Machine.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "The name of the Virtual Machine. " +
					"If not provided, a name is generated automatically.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"hostname": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "The hostname of the Virtual Machine. " +
					"If not provided, a hostname is generated.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"description": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "A description for the " +
					"Virtual Machine.",
				PlanModifiers: []planmodifier.String{
					PreserveEmptyStringStateForNullConfig(),
				},
			},
			"fqdn": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "The fully-qualified domain name of " +
					"the Virtual Machine.",
			},
			stateAttributeName: schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "The current state of the " +
					"Virtual Machine.",
			},
			"powered_on": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Whether the Virtual Machine should be " +
					"powered on. Set this explicitly to opt into ongoing power " +
					"state management; omit it to observe power state without " +
					"managing it. Powering off uses a graceful shutdown.",
			},
			"package": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Permalink or ID of a Virtual Machine " +
					"Package. Changing this will resize the Virtual Machine " +
					"to the new package in place. Note: Downgrades (to " +
					"packages with fewer vCPUs or memory) require the " +
					"Virtual Machine to be stopped. To stop and downgrade in " +
					"one apply, explicitly set `powered_on = false`; set it " +
					"to true in a later apply to start the VM again.",
				Validators: []validator.String{
					stringValidatorNotEmpty(),
				},
			},
			"disk_template": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Permalink or ID of the Disk " +
					"Template to use.",
				Validators: []validator.String{
					stringValidatorNotEmpty(),
				},
			},
			"disk_template_options": schema.MapAttribute{
				Optional:    true,
				ElementType: types.StringType,
				MarkdownDescription: "Options to pass to the Disk " +
					"Template during creation.",
			},
			"system_disk": schema.SingleNestedAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "The VM-owned boot disk. Additional disks " +
					"must use katapult_disk and katapult_disk_assignment.",
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.UseStateForUnknown(),
				},
				Attributes: map[string]schema.Attribute{
					"id": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "The unique identifier of the VM-owned system disk.",
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
					},
					"name": schema.StringAttribute{
						Optional:            true,
						Computed:            true,
						MarkdownDescription: "The name of the system disk.",
						Validators:          []validator.String{stringValidatorNotEmpty()},
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
					},
					"size_in_gb": schema.Int64Attribute{ //nolint:goconst // Terraform attribute name.
						Optional: true,
						Computed: true,
						MarkdownDescription: "Size of the system disk in GB, with a " +
							"minimum of 10 GB. Resizes are performed in place when the " +
							"selected method and VM power state permit it.",
						Validators:    []validator.Int64{int64validator.AtLeast(10)},
						PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
					},
					"resize_method": schema.StringAttribute{
						Optional: true,
						Computed: true,
						Default:  stringdefault.StaticString("offline"),
						MarkdownDescription: "Resize method: `offline` (the default) " +
							"requires the VM to be stopped and resizes the filesystem as " +
							"well as the disk; `online` permits growth while the VM runs " +
							"but resizes only the block device, so the guest filesystem " +
							"must be expanded manually. Shrink is always offline.",
						Validators: []validator.String{stringvalidator.OneOf("online", "offline")},
					},
					"wwn": schema.StringAttribute{ //nolint:goconst // Terraform attribute name.
						Computed:            true,
						MarkdownDescription: "World Wide Name identifier of the system disk.",
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
					},
					stateAttributeName: schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "Current state of the system disk.",
					},
				},
			},
			"ip_address_ids": schema.SetAttribute{
				Required:    true,
				ElementType: types.StringType,
				MarkdownDescription: "Set of IP address IDs to allocate " +
					"to the Virtual Machine.",
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
				},
			},
			"ip_addresses": schema.SetAttribute{
				Computed:    true,
				ElementType: types.StringType,
				MarkdownDescription: "Set of IP addresses allocated to " +
					"the Virtual Machine.",
			},
			"virtual_network_ids": schema.SetAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				MarkdownDescription: "Set of Virtual Network IDs to " +
					"attach to the Virtual Machine.",
				PlanModifiers: []planmodifier.Set{
					NullToEmptySetPlanModifier(),
				},
			},
			"network_speed_profile": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Permalink of the Network Speed " +
					"Profile to apply to all network interfaces.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"network_interfaces": schema.ListNestedAttribute{
				Computed: true,
				MarkdownDescription: "Network interface details for " +
					"the Virtual Machine.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed: true,
							MarkdownDescription: "The ID of the " +
								"network interface.",
						},
						"network_id": schema.StringAttribute{
							Computed: true,
							MarkdownDescription: "The ID of the " +
								"network the interface is on.",
						},
						"virtual_network_id": schema.StringAttribute{
							Computed: true,
							MarkdownDescription: "The ID of the virtual " +
								"network the interface is on.",
						},
						"mac_address": schema.StringAttribute{
							Computed: true,
							MarkdownDescription: "The MAC address of " +
								"the interface.",
						},
						"ip_addresses": schema.SetAttribute{
							Computed:    true,
							ElementType: types.StringType,
							MarkdownDescription: "The IP addresses " +
								"allocated to the interface.",
						},
					},
				},
			},
			"tags": schema.SetAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				MarkdownDescription: "Set of tag names to assign to the " +
					"Virtual Machine.",
				PlanModifiers: []planmodifier.Set{
					NullToEmptySetPlanModifier(),
				},
			},
			"group_id": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "The ID of the Virtual Machine Group " +
					"to assign this Virtual Machine to.",
				PlanModifiers: []planmodifier.String{
					PreserveEmptyStringStateForNullConfig(),
				},
			},
		},
		Blocks: map[string]schema.Block{
			"timeouts": timeouts.Block(ctx, timeouts.Opts{ //nolint:goconst // Terraform block name.
				Create: true,
				Update: true,
				UpdateDescription: "Maximum time Terraform waits for a Virtual " +
					"Machine update to complete. Defaults to 10 minutes. System disk " +
					"growth normally completes quickly, including filesystem-aware " +
					"offline growth. Offline system disk shrink may take substantially " +
					"longer; consider 4 hours or more for large disks. Reaching the " +
					"timeout stops Terraform waiting but does not cancel the Katapult " +
					"resize task, so check its state before retrying.",
				Delete: true,
			}),
			"disk": schema.ListNestedBlock{
				MarkdownDescription: "Deprecated creation-only disk list. The first " +
					"entry is the boot disk; migrate it to system_disk and each " +
					"additional entry to katapult_disk plus katapult_disk_assignment.",
				DeprecationMessage: "Use system_disk for the boot disk and katapult_disk with katapult_disk_assignment for additional disks.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Optional: true,
							MarkdownDescription: "Name of the disk. " +
								"Defaults to \"System Disk\" for " +
								"the first disk.",
						},
						virtualMachineDiskSizeAttribute: schema.Int64Attribute{
							Required:            true,
							MarkdownDescription: "Size of the disk in GB.",
						},
					},
				},
			},
		},
	}
}

//nolint:lll // Preserve complete operator-facing creation diagnostics.
func (r *VirtualMachineResource) Create( //nolint:funlen,gocyclo
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan VirtualMachineResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var configuredPoweredOn types.Bool
	resp.Diagnostics.Append(req.Config.GetAttribute(
		ctx, path.Root("powered_on"), &configuredPoweredOn,
	)...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := plan.Timeouts.Create(ctx, 20*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	spec := &buildspec.VirtualMachineSpec{
		DataCenter: &buildspec.DataCenter{
			Permalink: r.M.confDataCenter,
		},
		Hostname: r.M.UseOrGenerateHostname(plan.Hostname.ValueString()),
		AuthorizedKeys: &buildspec.AuthorizedKeys{
			AllSSHKeys: true,
			AllUsers:   true,
		},
	}

	if name := plan.Name.ValueString(); name != "" {
		spec.Name = name
	}
	if desc := plan.Description.ValueString(); desc != "" {
		spec.Description = desc
	}

	targetTags := plan.Tags
	if targetTags.IsUnknown() {
		resp.Diagnostics.Append(
			req.Config.GetAttribute(ctx, path.Root("tags"), &targetTags)...,
		)
	}
	planTags, diags := stringSetValueStrings(ctx, "tags", targetTags)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if len(planTags) > 0 {
		spec.Tags = planTags
	}

	pkgRef := plan.Package.ValueString()
	pkg := &buildspec.Package{}
	if strings.HasPrefix(pkgRef, "vmpkg_") {
		pkg.ID = pkgRef
	} else {
		pkg.Permalink = pkgRef
	}
	spec.Resources = &buildspec.Resources{Package: pkg}

	dtplRef := plan.DiskTemplate.ValueString()
	if dtplRef == "" {
		resp.Diagnostics.AddAttributeError(path.Root("disk_template"), "Missing Disk Template", "disk_template is required when creating a Virtual Machine")
		return
	}
	if strings.HasPrefix(dtplRef, "dtpl_") {
		spec.DiskTemplate = &buildspec.DiskTemplate{ID: dtplRef}
	} else {
		if !strings.Contains(dtplRef, "/") {
			dtplRef = "templates/" + dtplRef
		}
		spec.DiskTemplate = &buildspec.DiskTemplate{Permalink: dtplRef}
	}

	if !plan.DiskTemplateOptions.IsNull() &&
		!plan.DiskTemplateOptions.IsUnknown() {
		var opts map[string]string
		resp.Diagnostics.Append(
			plan.DiskTemplateOptions.ElementsAs(ctx, &opts, false)...,
		)
		if resp.Diagnostics.HasError() {
			return
		}
		for key, val := range opts {
			spec.DiskTemplate.Options = append(
				spec.DiskTemplate.Options,
				&buildspec.DiskTemplateOption{Key: key, Value: val},
			)
		}
	}

	if !plan.Disk.IsNull() && !plan.Disk.IsUnknown() {
		var disks []VirtualMachineDiskModel
		resp.Diagnostics.Append(
			plan.Disk.ElementsAs(ctx, &disks, false)...,
		)
		if resp.Diagnostics.HasError() {
			return
		}
		for i, d := range disks {
			diskName := d.Name.ValueString()
			if diskName == "" {
				if i == 0 {
					diskName = "System Disk"
				} else {
					diskName = fmt.Sprintf("Disk #%d", i+1)
				}
			}
			spec.SystemDisks = append(
				spec.SystemDisks,
				&buildspec.SystemDisk{
					Name: diskName,
					Size: int(d.Size.ValueInt64()),
				},
			)
		}
	}

	systemDisk, systemDiags := decodeVirtualMachineSystemDisk(ctx, plan.SystemDisk)
	resp.Diagnostics.Append(systemDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !plan.SystemDisk.IsNull() && !plan.SystemDisk.IsUnknown() &&
		!systemDisk.SizeInGB.IsNull() && !systemDisk.SizeInGB.IsUnknown() {
		diskName := systemDisk.Name.ValueString()
		if diskName == "" {
			diskName = "System Disk"
		}
		spec.SystemDisks = append(spec.SystemDisks, &buildspec.SystemDisk{
			Name: diskName,
			Size: int(systemDisk.SizeInGB.ValueInt64()),
		})
	}

	nspPermalink := plan.NetworkSpeedProfile.ValueString()
	var nsp *buildspec.NetworkSpeedProfile
	if nspPermalink != "" {
		nsp = &buildspec.NetworkSpeedProfile{Permalink: nspPermalink}
	}

	targetIPIDs := plan.IPAddressIDs
	if targetIPIDs.IsUnknown() {
		resp.Diagnostics.Append(
			req.Config.GetAttribute(
				ctx,
				path.Root("ip_address_ids"),
				&targetIPIDs,
			)...,
		)
	}
	ipIDs, diags := stringSetValueStrings(ctx, "ip_address_ids", targetIPIDs)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	ipGroups := map[string][]string{}
	for _, ipID := range ipIDs {
		id := ipID
		ipRes, err := r.M.Core.GetIpAddressWithResponse(ctx,
			&core.GetIpAddressParams{IpAddressId: &id})
		if err != nil {
			if ipRes != nil {
				err = genericAPIError(err, ipRes.Body)
			}
			resp.Diagnostics.AddError("Create Error", err.Error())
			return
		}
		if ipRes.JSON200 == nil {
			resp.Diagnostics.AddError(
				"Create Error",
				"unexpected empty response fetching IP",
			)
			return
		}
		ip := ipRes.JSON200.IpAddress
		if ip.Network == nil || ip.Network.Id == nil {
			resp.Diagnostics.AddError(
				"Create Error",
				fmt.Sprintf(
					"could not determine network of IP: %s", ipID,
				),
			)
			return
		}
		netID := *ip.Network.Id
		ipGroups[netID] = append(ipGroups[netID], ipID)
	}

	for netID, ips := range ipGroups {
		iface := &buildspec.NetworkInterface{
			Network: &buildspec.Network{ID: netID},
		}
		if nsp != nil {
			iface.SpeedProfile = nsp
		}
		for _, id := range ips {
			ipID := id
			iface.IPAddressAllocations = append(
				iface.IPAddressAllocations,
				&buildspec.IPAddressAllocation{
					Type: buildspec.ExistingIPAddressAllocation,
					IPAddress: &buildspec.IPAddress{
						ID: ipID,
					},
				},
			)
		}
		spec.NetworkInterfaces = append(spec.NetworkInterfaces, iface)
	}

	targetVnetIDs := plan.VirtualNetworkIDs
	if targetVnetIDs.IsUnknown() {
		resp.Diagnostics.Append(
			req.Config.GetAttribute(
				ctx,
				path.Root("virtual_network_ids"),
				&targetVnetIDs,
			)...,
		)
	}
	vnetIDs, diags := stringSetValueStrings(
		ctx, "virtual_network_ids", targetVnetIDs,
	)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if len(vnetIDs) > 0 {
		for _, vnID := range vnetIDs {
			iface := &buildspec.NetworkInterface{
				VirtualNetwork: &buildspec.VirtualNetwork{ID: vnID},
			}
			if nsp != nil {
				iface.SpeedProfile = nsp
			}
			spec.NetworkInterfaces = append(
				spec.NetworkInterfaces, iface,
			)
		}
	}

	if !plan.GroupID.IsNull() && plan.GroupID.ValueString() != "" {
		spec.Group = &buildspec.Group{
			ID: plan.GroupID.ValueString(),
		}
	}

	xmlBytes, err := spec.XML()
	if err != nil {
		resp.Diagnostics.AddError("Create Error", err.Error())
		return
	}
	xmlStr := string(xmlBytes)

	buildRes, err := r.M.Core.
		PostOrganizationVirtualMachinesBuildFromSpecWithResponse(ctx,
			core.PostOrganizationVirtualMachinesBuildFromSpecJSONRequestBody{
				Organization: core.OrganizationLookup{
					SubDomain: &r.M.confOrganization,
				},
				Xml: xmlStr,
			})
	if err != nil {
		if buildRes != nil {
			err = genericAPIError(err, buildRes.Body)
		}
		resp.Diagnostics.AddError("Create Error", err.Error())
		return
	}

	if buildRes.JSON201 == nil {
		resp.Diagnostics.AddError(
			"Create Error",
			"unexpected empty response from build",
		)
		return
	}
	buildID := buildRes.JSON201.VirtualMachineBuild.Id

	buildWaiter := &retry.StateChangeConf{
		Pending: []string{
			string(core.VirtualMachineBuildStateEnumDraft),
			string(core.VirtualMachineBuildStateEnumPending),
			string(core.VirtualMachineBuildStateEnumBuilding),
		},
		Target: []string{
			string(core.VirtualMachineBuildStateEnumComplete),
		},
		Refresh: func() (interface{}, string, error) {
			res, e := r.M.Core.
				GetVirtualMachinesBuildsVirtualMachineBuildWithResponse(
					ctx,
					&core.GetVirtualMachinesBuildsVirtualMachineBuildParams{
						VirtualMachineBuildId: buildID,
					},
				)
			if e != nil {
				if res != nil {
					e = genericAPIError(e, res.Body)
				}
				return nil, "", e
			}

			if res.JSON200 == nil {
				return nil, "", fmt.Errorf(
					"unexpected empty response polling build",
				)
			}
			b := res.JSON200.VirtualMachineBuild
			if b.State == nil {
				return b, "", fmt.Errorf("build state is nil")
			}
			if *b.State == core.VirtualMachineBuildStateEnumFailed {
				return b, string(*b.State),
					fmt.Errorf("virtual machine build failed")
			}

			return b, string(*b.State), nil
		},
		Timeout:                   timeout,
		Delay:                     r.M.stateChangeDelay(2 * time.Second),
		MinTimeout:                r.M.stateChangeDelay(5 * time.Second),
		PollInterval:              r.M.stateChangePollInterval(),
		ContinuousTargetOccurence: 1,
	}

	rawBuild, err := buildWaiter.WaitForStateContext(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Create Error",
			fmt.Sprintf(
				"error waiting for virtual machine build: %s", err,
			),
		)
		return
	}

	build := rawBuild.(core.GetVirtualMachinesBuildsVirtualMachineBuild200ResponseVirtualMachineBuild)
	vmPartial, err2 := build.VirtualMachine.Get()
	if err2 != nil || vmPartial.Id == nil {
		resp.Diagnostics.AddError(
			"Create Error",
			"build completed but virtual machine ID is not available",
		)
		return
	}
	vmID := *vmPartial.Id
	plan.ID = types.StringValue(vmID)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if len(planTags) > 0 {
		patchRes, e := r.M.Core.PatchVirtualMachineWithResponse(ctx,
			core.PatchVirtualMachineJSONRequestBody{
				VirtualMachine: core.VirtualMachineLookup{Id: &vmID},
				Properties: core.VirtualMachineArguments{
					TagNames: &planTags,
				},
			})
		if e != nil {
			if patchRes != nil {
				e = genericAPIError(e, patchRes.Body)
			}
			resp.Diagnostics.AddError("Create Error", e.Error())
			return
		}
	}

	vmWaiter := &retry.StateChangeConf{
		Pending: []string{
			string(core.Stopped),
			string(core.Allocating),
			string(core.Allocated),
			string(core.Starting),
			string(core.Migrating),
		},
		Target: []string{
			string(core.Started),
		},
		Refresh: func() (interface{}, string, error) {
			res, e := r.M.Core.GetVirtualMachineWithResponse(ctx,
				&core.GetVirtualMachineParams{
					VirtualMachineId: &vmID,
				})
			if e != nil {
				if res != nil {
					e = genericAPIError(e, res.Body)
				}
				return nil, "", e
			}
			if res.JSON200 == nil {
				return nil, "", fmt.Errorf(
					"unexpected empty response polling VM state",
				)
			}
			v := res.JSON200.VirtualMachine
			if v.State == nil {
				return v, "", fmt.Errorf("vm state is nil")
			}
			return v, string(*v.State), nil
		},
		Timeout:                   timeout,
		Delay:                     r.M.stateChangeDelay(2 * time.Second),
		MinTimeout:                r.M.stateChangeDelay(5 * time.Second),
		PollInterval:              r.M.stateChangePollInterval(),
		ContinuousTargetOccurence: 1,
	}

	_, err = vmWaiter.WaitForStateContext(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Create Error",
			fmt.Sprintf(
				"error waiting for virtual machine to start: %s", err,
			),
		)
		return
	}

	if !plan.SystemDisk.IsNull() && !plan.SystemDisk.IsUnknown() &&
		!systemDisk.Name.IsNull() && !systemDisk.Name.IsUnknown() &&
		(systemDisk.SizeInGB.IsNull() || systemDisk.SizeInGB.IsUnknown()) {
		bootDisk, bootErr := fetchVirtualMachineBootDisk(ctx, r.M, vmID, "")
		if bootErr != nil || bootDisk.Id == nil {
			resp.Diagnostics.AddError("Create Error", fmt.Sprintf("could not identify the boot disk for the requested rename: %v", bootErr))
			return
		}
		if err = patchDiskName(ctx, r.M, *bootDisk.Id, systemDisk.Name.ValueString()); err != nil {
			resp.Diagnostics.AddError("Create Error", err.Error())
			return
		}
	}

	if !configuredPoweredOn.IsNull() &&
		!configuredPoweredOn.IsUnknown() &&
		!configuredPoweredOn.ValueBool() {
		if err = reconcileVirtualMachinePowerState(
			ctx, r.M, vmID, false, timeout,
		); err != nil {
			resp.Diagnostics.AddError("Create Error", err.Error())
			return
		}
	}

	var attachments []core.GetVirtualMachineDisks200ResponseDisks
	if err := r.vmReadWithAttachments(ctx, &plan, &attachments); err != nil {
		resp.Diagnostics.AddError("Read Error", err.Error())
		return
	}
	if !plan.Disk.IsNull() && !plan.Disk.IsUnknown() && len(plan.Disk.Elements()) > 0 {
		legacyIDs, idsErr := virtualMachineDiskAttachmentIDs(attachments)
		if idsErr != nil {
			resp.Diagnostics.AddError("Create Error", idsErr.Error())
			return
		}
		encodedIDs, marshalErr := json.Marshal(legacyIDs)
		if marshalErr != nil {
			resp.Diagnostics.AddError("Create Error", marshalErr.Error())
			return
		}
		if resp.Private != nil {
			resp.Diagnostics.Append(resp.Private.SetKey(ctx, virtualMachineLegacyDiskIDsPrivateKey, encodedIDs)...)
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *VirtualMachineResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state VirtualMachineResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.vmRead(ctx, &state)
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

//nolint:lll // Preserve complete operator-facing update diagnostics.
func (r *VirtualMachineResource) Update( //nolint:funlen,gocyclo
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan VirtualMachineResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state VirtualMachineResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := plan.Timeouts.Update(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	vmID := state.ID.ValueString()
	plannedSystemResizeMethod := core.ResizeMethodEnum("")
	if req.Private != nil {
		encodedMethod, privateDiags := req.Private.GetKey(ctx, virtualMachineSystemDiskResizePrivateKey)
		resp.Diagnostics.Append(privateDiags...)
		if len(encodedMethod) > 0 {
			var method string
			if err := json.Unmarshal(encodedMethod, &method); err != nil {
				resp.Diagnostics.AddError("Invalid System Disk Resize Plan", err.Error())
				return
			}
			plannedSystemResizeMethod = core.ResizeMethodEnum(method)
		}
	}
	if resp.Diagnostics.HasError() {
		return
	}

	var configuredPoweredOn types.Bool
	resp.Diagnostics.Append(req.Config.GetAttribute(
		ctx, path.Root("powered_on"), &configuredPoweredOn,
	)...)
	if resp.Diagnostics.HasError() {
		return
	}
	powerManaged := !configuredPoweredOn.IsNull() &&
		!configuredPoweredOn.IsUnknown()
	plannedSystemDisk, systemDiags := decodeVirtualMachineSystemDisk(ctx, plan.SystemDisk)
	resp.Diagnostics.Append(systemDiags...)
	priorSystemDisk, priorSystemDiags := decodeVirtualMachineSystemDisk(ctx, state.SystemDisk)
	resp.Diagnostics.Append(priorSystemDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !plan.SystemDisk.IsNull() && !plan.SystemDisk.IsUnknown() &&
		(state.SystemDisk.IsNull() || state.SystemDisk.IsUnknown()) {
		bootDisk, bootErr := fetchVirtualMachineBootDisk(ctx, r.M, vmID, "")
		if bootErr != nil {
			resp.Diagnostics.AddAttributeError(path.Root("system_disk"), "Cannot Discover Boot Disk", bootErr.Error())
			return
		}
		resp.Diagnostics.Append(populateVirtualMachineSystemDisk(ctx, &state, bootDisk)...)
		priorSystemDisk, priorSystemDiags = decodeVirtualMachineSystemDisk(ctx, state.SystemDisk)
		resp.Diagnostics.Append(priorSystemDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
	}
	systemDiskResize := !plan.SystemDisk.IsNull() && !plan.SystemDisk.IsUnknown() &&
		!state.SystemDisk.IsNull() && !state.SystemDisk.IsUnknown() &&
		!plannedSystemDisk.SizeInGB.IsUnknown() &&
		!plannedSystemDisk.SizeInGB.Equal(priorSystemDisk.SizeInGB)
	coupledCtx := ctx
	if systemDiskResize {
		var cancel context.CancelFunc
		coupledCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	if powerManaged && !configuredPoweredOn.ValueBool() {
		if err := reconcileVirtualMachinePowerState(
			coupledCtx, r.M, vmID, false, timeout,
		); err != nil {
			resp.Diagnostics.AddError("Update Error", err.Error())
			return
		}
	}

	if !plan.Package.Equal(state.Package) {
		err := changeVirtualMachinePackage(
			ctx,
			r.M,
			vmID,
			plan.Package.ValueString(),
			timeout,
		)
		if err != nil {
			resp.Diagnostics.AddError("Update Error", err.Error())
			return
		}
	}

	if !plan.SystemDisk.IsNull() && !plan.SystemDisk.IsUnknown() &&
		!state.SystemDisk.IsNull() && !state.SystemDisk.IsUnknown() {
		diskID := priorSystemDisk.ID.ValueString()
		if diskID == "" {
			resp.Diagnostics.AddAttributeError(path.Root("system_disk"), "Unknown Boot Disk", "Cannot rename or resize the boot disk until its identity is authoritative.")
			return
		}
		nameChanged := !plannedSystemDisk.Name.IsUnknown() &&
			!plannedSystemDisk.Name.IsNull() &&
			!plannedSystemDisk.Name.Equal(priorSystemDisk.Name)
		sizeChanged := !plannedSystemDisk.SizeInGB.IsUnknown() &&
			!plannedSystemDisk.SizeInGB.Equal(priorSystemDisk.SizeInGB)
		if nameChanged || sizeChanged {
			currentBootDisk, err := fetchVirtualMachineBootDisk(coupledCtx, r.M, vmID, diskID)
			if err != nil {
				resp.Diagnostics.AddAttributeError(path.Root("system_disk"), "Cannot Validate Boot Disk", err.Error())
				return
			}
			if currentBootDisk.Id == nil || *currentBootDisk.Id != diskID {
				currentID := "<unavailable>"
				if currentBootDisk.Id != nil {
					currentID = *currentBootDisk.Id
				}
				resp.Diagnostics.AddAttributeError(path.Root("system_disk"), "System Disk State Changed After Planning", fmt.Sprintf("planned boot disk %s no longer matches the current boot disk %s; refresh state and run terraform plan again", diskID, currentID))
				return
			}
		}
		if nameChanged {
			if err := patchDiskName(coupledCtx, r.M, diskID, plannedSystemDisk.Name.ValueString()); err != nil {
				resp.Diagnostics.AddError("Update Error", err.Error())
				return
			}
		}
		if sizeChanged {
			vmState, err := fetchVirtualMachineState(coupledCtx, r.M, vmID)
			if err != nil {
				resp.Diagnostics.AddError("Update Error", err.Error())
				return
			}
			if vmState != core.Started && vmState != core.Stopped {
				resp.Diagnostics.AddAttributeError(path.Root("system_disk").AtName("size_in_gb"), "Cannot Resize System Disk", fmt.Sprintf("Virtual Machine is in transitional or unknown state %s; retry after it settles", vmState))
				return
			}
			attached := vmState == core.Started
			attachmentState := core.VirtualMachineDiskAttachmentStateEnumDetached
			if attached {
				attachmentState = core.VirtualMachineDiskAttachmentStateEnumAttached
			}
			method, err := effectiveDiskResizeMethod(
				priorSystemDisk.SizeInGB.ValueInt64(), plannedSystemDisk.SizeInGB.ValueInt64(),
				plannedSystemDisk.ResizeMethod.ValueString(), true, &attachmentState,
			)
			if err != nil {
				resp.Diagnostics.AddAttributeError(path.Root("system_disk").AtName("size_in_gb"), "Invalid System Disk Resize", err.Error())
				return
			}
			if plannedSystemResizeMethod != "" && plannedSystemResizeMethod != method {
				resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
				resp.Diagnostics.AddAttributeError(path.Root("system_disk").AtName("size_in_gb"), "System Disk State Changed After Planning", fmt.Sprintf("resize was planned as %s but the current VM power state requires %s; run terraform plan again", plannedSystemResizeMethod, method))
				return
			}
			if method == core.Online {
				resp.Diagnostics.AddAttributeWarning(path.Root("system_disk").AtName("size_in_gb"), "Online System Disk Growth", "Katapult grows the attached block device only; expand the guest partition and filesystem manually.")
			}
			if plannedSystemDisk.SizeInGB.ValueInt64() < priorSystemDisk.SizeInGB.ValueInt64() {
				resp.Diagnostics.AddAttributeWarning(path.Root("system_disk").AtName("size_in_gb"), "Offline System Disk Shrink", "Katapult must shrink the filesystem and partition before the disk; insufficient capacity fails the task.")
			}
			if err = resizeDiskAndWait(coupledCtx, r.M, diskID, plannedSystemDisk.SizeInGB.ValueInt64(), method, timeout); err != nil {
				observed := plan
				refreshCtx, cancelRefresh := context.WithTimeout(context.WithoutCancel(coupledCtx), 30*time.Second)
				defer cancelRefresh()
				if readErr := r.vmRead(refreshCtx, &observed); readErr == nil {
					resp.Diagnostics.Append(resp.State.Set(ctx, observed)...)
				} else {
					resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
				}
				resp.Diagnostics.AddError("Update Error", err.Error())
				return
			}
		}
	}

	args := core.VirtualMachineArguments{}

	if !plan.Name.IsUnknown() && !plan.Name.Equal(state.Name) {
		args.Name = plan.Name.ValueStringPointer()
	}
	if !plan.Hostname.IsUnknown() && !plan.Hostname.Equal(state.Hostname) {
		args.Hostname = plan.Hostname.ValueStringPointer()
	}
	if !plan.Description.IsUnknown() &&
		!plan.Description.Equal(state.Description) {
		if plan.Description.IsNull() {
			emptyDescription := ""
			args.Description = &emptyDescription
		} else {
			args.Description = plan.Description.ValueStringPointer()
		}
	}
	targetTags := plan.Tags
	if targetTags.IsUnknown() {
		resp.Diagnostics.Append(
			req.Config.GetAttribute(ctx, path.Root("tags"), &targetTags)...,
		)
	}
	if !targetTags.IsUnknown() && !targetTags.Equal(state.Tags) {
		tags, diags := stringSetValueStrings(ctx, "tags", targetTags)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		args.TagNames = &tags
	}

	// Detect whether the user explicitly removed group_id from config
	// (config is null) while state still has a group set.
	var configGroupID types.String
	resp.Diagnostics.Append(
		req.Config.GetAttribute(ctx, path.Root("group_id"), &configGroupID)...,
	)
	if resp.Diagnostics.HasError() {
		return
	}
	clearGroup := configGroupID.IsNull() &&
		!state.GroupID.IsNull() &&
		state.GroupID.ValueString() != ""
	setGroup := !plan.GroupID.IsNull() &&
		!plan.GroupID.IsUnknown() &&
		!plan.GroupID.Equal(state.GroupID) &&
		plan.GroupID.ValueString() != ""

	// Build a custom PATCH body so we can send "group": null when clearing.
	props := vmGroupPatchProperties{
		VirtualMachineArguments: args,
	}

	switch {
	case clearGroup:
		nullGroup := json.RawMessage(`null`)
		props.Group = &nullGroup
	case !plan.GroupID.IsNull() &&
		!plan.GroupID.IsUnknown() &&
		!plan.GroupID.Equal(state.GroupID) &&
		plan.GroupID.ValueString() == "":
		nullGroup := json.RawMessage(`null`)
		props.Group = &nullGroup
	case setGroup:
		groupBytes, _ := json.Marshal(
			core.VirtualMachineGroupLookup{
				Id: plan.GroupID.ValueStringPointer(),
			},
		)
		rg := json.RawMessage(groupBytes)
		props.Group = &rg
	}

	shouldPatch := args.Name != nil || args.Hostname != nil ||
		args.Description != nil || args.TagNames != nil || props.Group != nil
	if shouldPatch {
		patchBodyBytes, marshalErr := json.Marshal(vmGroupPatchBody{
			VirtualMachine: core.VirtualMachineLookup{Id: &vmID},
			Properties:     props,
		})
		if marshalErr != nil {
			resp.Diagnostics.AddError("Update Error", marshalErr.Error())
			return
		}

		patchRes, err := r.M.Core.PatchVirtualMachineWithBodyWithResponse(
			ctx, "application/json", bytes.NewReader(patchBodyBytes),
		)
		if err != nil {
			if patchRes != nil {
				err = genericAPIError(err, patchRes.Body)
			}
			resp.Diagnostics.AddError("Update Error", err.Error())
			return
		}
	}

	if !plan.IPAddressIDs.Equal(state.IPAddressIDs) {
		targetIPIDs := plan.IPAddressIDs
		if targetIPIDs.IsUnknown() {
			resp.Diagnostics.Append(
				req.Config.GetAttribute(
					ctx,
					path.Root("ip_address_ids"),
					&targetIPIDs,
				)...,
			)
		}

		targetIDs, diags := stringSetValueStrings(
			ctx, "ip_address_ids", targetIPIDs,
		)
		resp.Diagnostics.Append(diags...)

		stateIDs, diags := stringSetValueStrings(
			ctx, "ip_address_ids", state.IPAddressIDs,
		)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}

		addIDs := stringsDiff(targetIDs, stateIDs)
		removeIDs := stringsDiff(stateIDs, targetIDs)

		if err := allocateIPsToVM(ctx, r.M, vmID, addIDs); err != nil {
			resp.Diagnostics.AddError("Update Error", err.Error())
			return
		}

		for _, ipID := range removeIDs {
			id := ipID
			_, e := r.M.Core.PostIpAddressUnallocateWithResponse(ctx,
				core.PostIpAddressUnallocateJSONRequestBody{
					IpAddress: core.IPAddressLookup{Id: &id},
				})
			if e != nil && !errors.Is(e, core.ErrNotFound) {
				resp.Diagnostics.AddError("Update Error", e.Error())
				return
			}
		}
	}

	targetVnetIDs := plan.VirtualNetworkIDs
	if targetVnetIDs.IsUnknown() {
		resp.Diagnostics.Append(
			req.Config.GetAttribute(
				ctx,
				path.Root("virtual_network_ids"),
				&targetVnetIDs,
			)...,
		)
	}
	if !targetVnetIDs.IsUnknown() &&
		!targetVnetIDs.Equal(state.VirtualNetworkIDs) {
		targetIDs, diags := stringSetValueStrings(
			ctx, "virtual_network_ids", targetVnetIDs,
		)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}

		ifaces, err := fetchAllVMNetworkInterfaces(ctx, r.M, vmID)
		if err != nil {
			resp.Diagnostics.AddError("Update Error", err.Error())
			return
		}

		attachedVnetIDs := make([]string, 0)
		detachedVnets := make(map[string]string)
		for _, iface := range ifaces {
			if !iface.VirtualNetwork.IsSpecified() || iface.Id == nil {
				continue
			}
			vnet, err2 := iface.VirtualNetwork.Get()
			if err2 != nil || vnet.Id == nil {
				continue
			}
			if iface.State == nil {
				continue
			}
			switch *iface.State {
			case "attached": //nolint:goconst // API state value.
				attachedVnetIDs = append(
					attachedVnetIDs, *vnet.Id,
				)
			case "detached":
				detachedVnets[*vnet.Id] = *iface.Id
			}
		}

		missingVnetIDs := stringsDiff(targetIDs, attachedVnetIDs)
		removeVnetIDs := stringsDiff(attachedVnetIDs, targetIDs)

		var addVnetIDs, attachIfaceIDs []string
		for _, id := range missingVnetIDs {
			if ifaceID, ok := detachedVnets[id]; ok {
				attachIfaceIDs = append(attachIfaceIDs, ifaceID)
			} else {
				addVnetIDs = append(addVnetIDs, id)
			}
		}

		nsp := plan.NetworkSpeedProfile.ValueString()
		for _, vnID := range addVnetIDs {
			if e := addVirtualNetworkToVM(
				ctx, r.M, vmID, vnID, nsp, timeout,
			); e != nil {
				resp.Diagnostics.AddError("Update Error", e.Error())
				return
			}
		}

		for _, ifaceID := range attachIfaceIDs {
			if e := attachVMNetworkInterface(
				ctx, r.M, ifaceID, timeout,
			); e != nil {
				resp.Diagnostics.AddError("Update Error", e.Error())
				return
			}
		}

		var removeIfaceIDs []string
		for _, id := range removeVnetIDs {
			for _, iface := range ifaces {
				if !iface.VirtualNetwork.IsSpecified() ||
					iface.Id == nil {
					continue
				}
				vnet, err2 := iface.VirtualNetwork.Get()
				if err2 != nil || vnet.Id == nil {
					continue
				}
				if *vnet.Id == id {
					removeIfaceIDs = append(
						removeIfaceIDs, *iface.Id,
					)
				}
			}
		}

		for _, ifaceID := range removeIfaceIDs {
			if e := removeVMNetworkInterface(
				ctx, r.M, ifaceID, timeout,
			); e != nil {
				resp.Diagnostics.AddError("Update Error", e.Error())
				return
			}
		}
	}

	if !plan.NetworkSpeedProfile.IsUnknown() &&
		!plan.NetworkSpeedProfile.Equal(state.NetworkSpeedProfile) {
		permalink := plan.NetworkSpeedProfile.ValueString()
		if e := updateVMNetworkSpeedProfile(
			ctx, r.M, vmID, permalink, timeout,
		); e != nil {
			resp.Diagnostics.AddError("Update Error", e.Error())
			return
		}
	}

	if powerManaged && configuredPoweredOn.ValueBool() {
		if err := reconcileVirtualMachinePowerState(
			coupledCtx, r.M, vmID, true, timeout,
		); err != nil {
			resp.Diagnostics.AddError("Update Error", err.Error())
			return
		}
	}

	if err := r.vmRead(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Read Error", err.Error())
		return
	}
	if resp.Private != nil {
		resp.Diagnostics.Append(resp.Private.SetKey(ctx, virtualMachineSystemDiskResizePrivateKey, nil)...)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

//nolint:lll // Preserve complete relationship ownership diagnostics.
func (r *VirtualMachineResource) Delete( //nolint:funlen,gocyclo
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state VirtualMachineResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := state.Timeouts.Delete(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	vmID := state.ID.ValueString()

	attachments, err := fetchAllVMDisks(ctx, r.M, vmID)
	if err != nil && !errors.Is(err, core.ErrNotFound) {
		resp.Diagnostics.AddError("Delete Error", fmt.Sprintf("checking VM disk relationships: %s", err))
		return
	}
	legacyCount := 0
	if !state.Disk.IsNull() && !state.Disk.IsUnknown() {
		legacyCount = len(state.Disk.Elements())
	}
	if legacyCount > 0 {
		if ownershipErr := validateLegacyDiskDeleteOwnership(
			ctx, req.Private, attachments, state.SystemDisk,
		); ownershipErr != nil {
			resp.Diagnostics.AddError("Delete Error", ownershipErr.Error())
			return
		}
	} else if len(attachments) > 0 {
		priorSystem, diags := decodeVirtualMachineSystemDisk(ctx, state.SystemDisk)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		boot, authoritative := selectBootDiskAssignment(attachments, priorSystem.ID.ValueString())
		if !authoritative || boot == nil || boot.Disk == nil || boot.Disk.Id == nil {
			resp.Diagnostics.AddError("Delete Error", "cannot identify the boot relationship; refusing to cascade unknown disk relationships")
			return
		}
		if len(attachments) > 1 {
			resp.Diagnostics.AddError("Delete Error", "Virtual Machine still has non-boot disk relationships; remove the corresponding katapult_disk_assignment resources first")
			return
		}
	}

	vmRes, err := r.M.Core.GetVirtualMachineWithResponse(ctx,
		&core.GetVirtualMachineParams{VirtualMachineId: &vmID})
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return
		}
		if vmRes != nil && isErrNotFoundOrInTrash(err, vmRes.JSON406) {
			if r.M.SkipTrashObjectPurge {
				return
			}

			err = purgeTrashObjectByObjectID(ctx, r.M, timeout, vmID)
			if err != nil && !isErrNotFoundOrInTrash(err, nil) {
				resp.Diagnostics.AddError(
					"Delete Error",
					fmt.Sprintf("failed to purge VM from trash: %s", err),
				)
			}
			return
		}
		if vmRes != nil {
			err = genericAPIError(err, vmRes.Body)
		}
		resp.Diagnostics.AddError("Delete Error", err.Error())
		return
	}
	if vmRes.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Delete Error",
			"unexpected empty response fetching VM",
		)
		return
	}
	vm := vmRes.JSON200.VirtualMachine

	if vm.State == nil {
		resp.Diagnostics.AddError(
			"Delete Error", "virtual machine state is nil",
		)
		return
	}

	stopped := false
	switch *vm.State { //nolint:exhaustive
	case core.Started:
		stopRes, e := r.M.Core.PostVirtualMachineStopWithResponse(ctx,
			core.PostVirtualMachineStopJSONRequestBody{
				VirtualMachine: core.VirtualMachineLookup{Id: &vmID},
			})
		if e != nil {
			if stopRes != nil {
				e = genericAPIError(e, stopRes.Body)
			}
			if !isErrNotFoundOrInTrash(e, nil) {
				resp.Diagnostics.AddError(
					"Delete Error",
					fmt.Sprintf("failed to stop VM: %s", e),
				)
				return
			}
		}

		if e == nil && stopRes != nil && stopRes.JSON200 != nil &&
			stopRes.JSON200.Task.Id != nil {
			taskID := *stopRes.JSON200.Task.Id
			e = waitForTaskCompletion(ctx, r.M, timeout, taskID)
			if e != nil && !isErrNotFoundOrInTrash(e, nil) {
				resp.Diagnostics.AddError(
					"Delete Error",
					fmt.Sprintf("failed to stop VM: %s", e),
				)
				return
			}
		}
	case core.Stopping, core.ShuttingDown:
		// Wait for the VM to stop below.
	case core.Stopped:
		stopped = true
	default:
		resp.Diagnostics.AddError(
			"Delete Error",
			fmt.Sprintf(
				"cannot delete VM in state: %s", string(*vm.State),
			),
		)
		return
	}

	if !stopped {
		err = waitForVMToStop(ctx, r.M, vmID, timeout)
		if err != nil && !isErrNotFoundOrInTrash(err, nil) {
			resp.Diagnostics.AddError(
				"Delete Error",
				fmt.Sprintf("failed to stop VM: %s", err),
			)
			return
		}
	}

	if r.M.SkipTrashObjectPurge {
		if _, e := addVMUniqueHostnameSuffix(
			ctx, r.M, vmID, vm.Hostname,
		); e != nil && !isErrNotFoundOrInTrash(e, nil) {
			resp.Diagnostics.AddError(
				"Delete Error",
				fmt.Sprintf(
					"failed to update VM hostname before trash: %s",
					e,
				),
			)
			return
		}
	}

	delRes, err := r.M.Core.DeleteVirtualMachineWithResponse(ctx,
		core.DeleteVirtualMachineJSONRequestBody{
			VirtualMachine: &core.VirtualMachineLookup{Id: &vmID},
		})
	deleteReturnedInTrash := false
	if err != nil {
		if delRes == nil {
			resp.Diagnostics.AddError(
				"Delete Error",
				fmt.Sprintf("failed to delete VM: %s", err),
			)
			return
		}

		if errors.Is(err, core.ErrNotFound) {
			return
		}
		if isErrNotFoundOrInTrash(err, delRes.JSON406) {
			deleteReturnedInTrash = true
		} else {
			err = genericAPIError(err, delRes.Body)
			resp.Diagnostics.AddError(
				"Delete Error",
				fmt.Sprintf("failed to delete VM: %s", err),
			)
			return
		}
	}

	var ipIDs []string
	resp.Diagnostics.Append(
		state.IPAddressIDs.ElementsAs(ctx, &ipIDs, false)...,
	)
	if resp.Diagnostics.HasError() {
		return
	}

	for _, ipID := range ipIDs {
		id := ipID
		_, e := r.M.Core.PostIpAddressUnallocateWithResponse(ctx,
			core.PostIpAddressUnallocateJSONRequestBody{
				IpAddress: core.IPAddressLookup{Id: &id},
			})
		if e != nil && !errors.Is(e, core.ErrNotFound) {
			resp.Diagnostics.AddError(
				"Delete Error",
				fmt.Sprintf(
					"failed to unallocate IP %s: %s", ipID, e,
				),
			)
			return
		}
	}

	if !r.M.SkipTrashObjectPurge {
		var e error
		switch {
		case deleteReturnedInTrash:
			e = purgeTrashObjectByObjectID(ctx, r.M, timeout, vmID)
		case delRes != nil && delRes.JSON200 != nil:
			e = purgeTrashObject(ctx, r.M, timeout, delRes.JSON200.TrashObject)
		}
		if e != nil && !isErrNotFoundOrInTrash(e, nil) {
			resp.Diagnostics.AddError(
				"Delete Error",
				fmt.Sprintf("failed to purge VM from trash: %s", e),
			)
			return
		}
	}
}

func (r *VirtualMachineResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
	resp.Diagnostics.Append(resp.Private.SetKey(ctx, virtualMachineImportDiskTemplatePrivateKey, []byte("true"))...)
	resp.Diagnostics.Append(resp.Private.SetKey(ctx, virtualMachineImportTemplateOptionsPrivateKey, []byte("true"))...)
}

func virtualMachineDiskAttachmentIDs(
	attachments []core.GetVirtualMachineDisks200ResponseDisks,
) ([]string, error) {
	ids := make([]string, 0, len(attachments))
	for i := range attachments {
		if attachments[i].Disk == nil || attachments[i].Disk.Id == nil || *attachments[i].Disk.Id == "" {
			return nil, fmt.Errorf("disk relationship %d has no disk identity", i)
		}
		ids = append(ids, *attachments[i].Disk.Id)
	}
	sort.Strings(ids)
	return ids, nil
}

func validateLegacyDiskDeleteOwnership(
	ctx context.Context,
	private interface {
		GetKey(context.Context, string) ([]byte, diag.Diagnostics)
	},
	attachments []core.GetVirtualMachineDisks200ResponseDisks,
	systemDisk types.Object,
) error {
	remoteIDs, err := virtualMachineDiskAttachmentIDs(attachments)
	if err != nil {
		return fmt.Errorf("cannot verify deprecated disk ownership: %w", err)
	}
	var encoded []byte
	if private != nil {
		var diags diag.Diagnostics
		encoded, diags = private.GetKey(ctx, virtualMachineLegacyDiskIDsPrivateKey)
		if diags.HasError() {
			return fmt.Errorf("reading deprecated disk ownership state: %s", diags)
		}
	}
	if len(encoded) == 0 {
		if len(attachments) == 0 {
			return nil
		}
		priorSystem, diags := decodeVirtualMachineSystemDisk(ctx, systemDisk)
		if diags.HasError() {
			return fmt.Errorf("reading system disk identity: %s", diags)
		}
		boot, authoritative := selectBootDiskAssignment(attachments, priorSystem.ID.ValueString())
		if len(attachments) == 1 && authoritative && boot != nil {
			return nil
		}
		return fmt.Errorf(
			"cannot prove ownership of deprecated additional disk relationships; " +
				"migrate them to katapult_disk and katapult_disk_assignment before " +
				"deleting the Virtual Machine",
		)
	}
	var ownedIDs []string
	if err := json.Unmarshal(encoded, &ownedIDs); err != nil {
		return fmt.Errorf("decoding deprecated disk ownership state: %w", err)
	}
	owned := make(map[string]struct{}, len(ownedIDs))
	for _, id := range ownedIDs {
		if id == "" {
			return fmt.Errorf("deprecated disk ownership state contains an empty disk identity")
		}
		owned[id] = struct{}{}
	}
	for _, id := range remoteIDs {
		if _, ok := owned[id]; !ok {
			return fmt.Errorf(
				"virtual machine has disk relationship %s which is not owned by "+
					"deprecated disk state; remove its katapult_disk_assignment first",
				id,
			)
		}
	}
	return nil
}

func (r *VirtualMachineResource) vmRead(
	ctx context.Context,
	model *VirtualMachineResourceModel,
) error {
	return r.vmReadWithAttachments(ctx, model, nil)
}

//nolint:gocyclo,funlen
func (r *VirtualMachineResource) vmReadWithAttachments(
	ctx context.Context,
	model *VirtualMachineResourceModel,
	attachmentsOut *[]core.GetVirtualMachineDisks200ResponseDisks,
) error {
	vmID := model.ID.ValueString()

	vmRes, err := r.M.Core.GetVirtualMachineWithResponse(ctx,
		&core.GetVirtualMachineParams{VirtualMachineId: &vmID})
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return err
		}
		if vmRes != nil && isErrNotFoundOrInTrash(err, vmRes.JSON406) {
			return core.ErrNotFound
		}
		if vmRes != nil {
			err = genericAPIError(err, vmRes.Body)
		}
		return err
	}
	if vmRes.JSON200 == nil {
		return fmt.Errorf("unexpected empty response fetching VM")
	}
	vm := vmRes.JSON200.VirtualMachine

	priorSystem, systemDiags := decodeVirtualMachineSystemDisk(ctx, model.SystemDisk)
	if systemDiags.HasError() {
		return fmt.Errorf("reading system_disk state: %s", systemDiags)
	}
	bootDisk, attachments, bootErr := fetchVirtualMachineBootDiskWithAttachments(
		ctx, r.M, vmID, priorSystem.ID.ValueString(),
	)
	if attachmentsOut != nil {
		*attachmentsOut = attachments
	}
	if errors.Is(bootErr, errNoVirtualMachineBootDisk) {
		model.SystemDisk = types.ObjectNull(virtualMachineSystemDiskAttrTypes)
	} else if bootErr != nil {
		return fmt.Errorf("discovering boot disk: %w", bootErr)
	} else if diags := populateVirtualMachineSystemDisk(ctx, model, bootDisk); diags.HasError() {
		return fmt.Errorf("populating system_disk state: %s", diags)
	}
	if bootDisk != nil && bootDisk.Installation.IsSpecified() && !bootDisk.Installation.IsNull() {
		if installation, e := bootDisk.Installation.Get(); e == nil &&
			installation.DiskTemplateVersion != nil &&
			installation.DiskTemplateVersion.DiskTemplate != nil {
			template := installation.DiskTemplateVersion.DiskTemplate
			configured := model.DiskTemplate.ValueString()
			switch {
			case strings.HasPrefix(configured, "dtpl_") && template.Id != nil:
				model.DiskTemplate = types.StringValue(*template.Id)
			case configured != "" && template.Permalink != nil &&
				(*template.Permalink == configured || *template.Permalink == "templates/"+configured):
				model.DiskTemplate = types.StringValue(configured)
			case template.Permalink != nil:
				model.DiskTemplate = types.StringValue(*template.Permalink)
			case template.Id != nil:
				model.DiskTemplate = types.StringValue(*template.Id)
			}
		}
	}

	ifaces, err := fetchAllVMNetworkInterfaces(ctx, r.M, vmID)
	if err != nil {
		return err
	}

	vnetIDs := make([]attr.Value, 0)
	for _, iface := range ifaces {
		if !iface.VirtualNetwork.IsSpecified() || iface.Id == nil {
			continue
		}
		vnet, err2 := iface.VirtualNetwork.Get()
		if err2 != nil || vnet.Id == nil {
			continue
		}
		if iface.State == nil || *iface.State != "attached" {
			continue
		}
		vnetIDs = append(vnetIDs, types.StringValue(*vnet.Id))
	}

	var nsp string
	if len(ifaces) > 0 && ifaces[0].SpeedProfile != nil &&
		ifaces[0].SpeedProfile.Permalink != nil {
		nsp = *ifaces[0].SpeedProfile.Permalink
	}

	model.Name = types.StringPointerValue(vm.Name)
	// The API normalizes hostnames to lowercase. Preserve the case already
	// in the model (from plan or state) when it differs only by case, so
	// the state stays consistent with the config value the user specified.
	apiHostname := ""
	if vm.Hostname != nil {
		apiHostname = *vm.Hostname
	}
	if model.Hostname.IsNull() || model.Hostname.IsUnknown() ||
		!strings.EqualFold(model.Hostname.ValueString(), apiHostname) {
		model.Hostname = types.StringValue(apiHostname)
	}
	model.FQDN = types.StringPointerValue(vm.Fqdn)

	if vm.State != nil {
		populateVirtualMachinePowerState(model, *vm.State)
	}

	if desc, err2 := vm.Description.Get(); err2 == nil && desc != "" {
		model.Description = types.StringValue(desc)
	} else {
		model.Description = optionalOnlyStringAbsentValue(model.Description)
	}

	if vm.Package.IsSpecified() {
		if pkg, err2 := vm.Package.Get(); err2 == nil {
			configuredPkg := ""
			if !model.Package.IsNull() && !model.Package.IsUnknown() {
				configuredPkg = model.Package.ValueString()
			}
			normalizedPkg := normalizeVirtualMachinePackageForState(
				configuredPkg,
				pkg,
			)
			if normalizedPkg != "" {
				model.Package = types.StringValue(normalizedPkg)
			}
		}
	}

	if vm.Group.IsSpecified() {
		if grp, err2 := vm.Group.Get(); err2 == nil && grp.Id != nil {
			model.GroupID = types.StringPointerValue(grp.Id)
		} else {
			model.GroupID = optionalOnlyStringAbsentValue(model.GroupID)
		}
	} else {
		model.GroupID = optionalOnlyStringAbsentValue(model.GroupID)
	}

	if nsp != "" {
		model.NetworkSpeedProfile = types.StringValue(nsp)
	} else {
		model.NetworkSpeedProfile = types.StringNull()
	}

	if vm.IpAddresses != nil {
		ipIDs := make([]attr.Value, 0, len(*vm.IpAddresses))
		ipAddrs := make([]attr.Value, 0, len(*vm.IpAddresses))
		for _, ip := range *vm.IpAddresses {
			if ip.Id != nil {
				ipIDs = append(ipIDs, types.StringValue(*ip.Id))
			}
			if ip.Address != nil {
				ipAddrs = append(
					ipAddrs, types.StringValue(*ip.Address),
				)
			}
		}
		model.IPAddressIDs = types.SetValueMust(
			types.StringType, ipIDs,
		)
		model.IPAddresses = types.SetValueMust(
			types.StringType, ipAddrs,
		)
	} else {
		model.IPAddressIDs = types.SetValueMust(
			types.StringType, make([]attr.Value, 0),
		)
		model.IPAddresses = types.SetValueMust(
			types.StringType, make([]attr.Value, 0),
		)
	}

	model.VirtualNetworkIDs = types.SetValueMust(
		types.StringType, vnetIDs,
	)

	tagVals := make([]attr.Value, 0)
	if vm.TagNames != nil {
		for _, t := range *vm.TagNames {
			tagVals = append(tagVals, types.StringValue(t))
		}
	}
	model.Tags = types.SetValueMust(types.StringType, tagVals)

	niList, err := buildVMNetworkInterfaceList(ifaces)
	if err != nil {
		return err
	}
	model.NetworkInterfaces = niList

	return nil
}

func optionalOnlyStringAbsentValue(current types.String) types.String {
	if !current.IsNull() && !current.IsUnknown() &&
		current.ValueString() == "" {
		return types.StringValue("")
	}

	return types.StringNull()
}

func virtualMachinePoweredOnProjection(
	state core.VirtualMachineStateEnum,
	previous types.Bool,
) types.Bool {
	switch state { //nolint:exhaustive
	case core.Started, core.Starting:
		return types.BoolValue(true)
	case core.Stopped, core.Stopping, core.ShuttingDown:
		return types.BoolValue(false)
	default:
		if !previous.IsNull() && !previous.IsUnknown() {
			return previous
		}

		return types.BoolNull()
	}
}

func populateVirtualMachinePowerState(
	model *VirtualMachineResourceModel,
	state core.VirtualMachineStateEnum,
) {
	model.State = types.StringValue(string(state))
	model.PoweredOn = virtualMachinePoweredOnProjection(state, model.PoweredOn)
}

func reconcileVirtualMachinePowerState(
	ctx context.Context,
	m *Meta,
	vmID string,
	poweredOn bool,
	timeout time.Duration,
) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	target := core.Stopped
	if poweredOn {
		target = core.Started
	}

	for {
		state, err := fetchVirtualMachineState(ctx, m, vmID)
		if err != nil {
			return fmt.Errorf(
				"failed to fetch virtual machine %s while reconciling to %s: %w",
				vmID, target, err,
			)
		}

		if state == target {
			return nil
		}

		switch state {
		case core.Failed, core.Orphaned:
			return fmt.Errorf(
				"cannot reconcile virtual machine %s from terminal state %s to %s",
				vmID, state, target,
			)
		case core.Starting, core.Stopping, core.ShuttingDown:
			if err = waitForVirtualMachineStableState(
				ctx, m, vmID, timeout,
			); err != nil {
				return fmt.Errorf(
					"error waiting for virtual machine %s to settle from %s: %w",
					vmID, state, err,
				)
			}
			continue
		case core.Allocated, core.Allocating, core.Resetting, core.Migrating,
			core.Transferring:
			if err = waitForVirtualMachineStableState(
				ctx, m, vmID, timeout,
			); err != nil {
				return fmt.Errorf(
					"error waiting for virtual machine %s to settle from %s: %w",
					vmID, state, err,
				)
			}
			continue
		case core.Started, core.Stopped:
			// Queue the required action below.
		default:
			return fmt.Errorf(
				"cannot reconcile virtual machine %s from unsupported state %q to %s",
				vmID, state, target,
			)
		}

		taskID, action, err := queueVirtualMachinePowerAction(
			ctx, m, vmID, poweredOn,
		)
		if err != nil {
			return err
		}
		if err = waitForTaskCompletion(ctx, m, timeout, taskID); err != nil {
			return fmt.Errorf(
				"error waiting for virtual machine %s %s task: %w",
				vmID, action, err,
			)
		}
		if err = waitForVirtualMachineState(
			ctx,
			m,
			vmID,
			virtualMachineExactStatePending(target),
			[]core.VirtualMachineStateEnum{target},
			timeout,
		); err != nil {
			return virtualMachineStateWaitError(vmID, state, target, err)
		}

		return nil
	}
}

func queueVirtualMachinePowerAction(
	ctx context.Context,
	m *Meta,
	vmID string,
	poweredOn bool,
) (taskID string, action string, err error) {
	lookup := core.VirtualMachineLookup{Id: &vmID}
	if poweredOn {
		action = "start"
		res, actionErr := m.Core.PostVirtualMachineStartWithResponse(
			ctx,
			core.PostVirtualMachineStartJSONRequestBody{VirtualMachine: lookup},
		)
		if actionErr != nil {
			if res != nil {
				actionErr = genericAPIError(actionErr, res.Body)
			}
			return "", action, fmt.Errorf(
				"failed to queue start for virtual machine %s: %w",
				vmID, actionErr,
			)
		}
		if res == nil || res.JSON200 == nil || res.JSON200.Task.Id == nil ||
			*res.JSON200.Task.Id == "" {
			return "", action, fmt.Errorf(
				"unexpected empty task response queueing start for virtual machine %s",
				vmID,
			)
		}

		return *res.JSON200.Task.Id, action, nil
	}

	action = virtualMachineShutdownActionLabel
	res, actionErr := m.Core.PostVirtualMachineShutdownWithResponse(
		ctx,
		core.PostVirtualMachineShutdownJSONRequestBody{VirtualMachine: lookup},
	)
	if actionErr != nil {
		if res != nil {
			actionErr = genericAPIError(actionErr, res.Body)
		}
		return "", action, fmt.Errorf(
			"failed to queue graceful shutdown for virtual machine %s: %w",
			vmID, actionErr,
		)
	}
	if res == nil || res.JSON200 == nil || res.JSON200.Task.Id == nil ||
		*res.JSON200.Task.Id == "" {
		return "", action, fmt.Errorf(
			"unexpected empty task response queueing graceful shutdown for virtual machine %s",
			vmID,
		)
	}

	return *res.JSON200.Task.Id, action, nil
}

func fetchVirtualMachineState(
	ctx context.Context,
	m *Meta,
	vmID string,
) (core.VirtualMachineStateEnum, error) {
	res, err := m.Core.GetVirtualMachineWithResponse(
		ctx,
		&core.GetVirtualMachineParams{VirtualMachineId: &vmID},
	)
	if err != nil {
		if res != nil {
			err = genericAPIError(err, res.Body)
		}
		return "", err
	}
	if res == nil || res.JSON200 == nil {
		return "", fmt.Errorf("unexpected empty response polling VM state")
	}
	if res.JSON200.VirtualMachine.State == nil {
		return "", fmt.Errorf("virtual machine state is nil")
	}

	return *res.JSON200.VirtualMachine.State, nil
}

func waitForVirtualMachineState(
	ctx context.Context,
	m *Meta,
	vmID string,
	pending []core.VirtualMachineStateEnum,
	target []core.VirtualMachineStateEnum,
	timeout time.Duration,
) error {
	waiter := &retry.StateChangeConf{
		Pending:                   virtualMachineStateStrings(pending),
		Target:                    virtualMachineStateStrings(target),
		Timeout:                   timeout,
		Delay:                     m.stateChangeDelay(1 * time.Second),
		MinTimeout:                m.stateChangeDelay(5 * time.Second),
		PollInterval:              m.stateChangePollInterval(),
		ContinuousTargetOccurence: 1,
	}
	waiter.Refresh = func() (interface{}, string, error) {
		state, err := fetchVirtualMachineState(ctx, m, vmID)
		if err != nil {
			return nil, "", err
		}

		return state, string(state), nil
	}

	raw, err := waiter.WaitForStateContext(ctx)
	if err != nil {
		return err
	}
	_, ok := raw.(core.VirtualMachineStateEnum)
	if !ok {
		return fmt.Errorf("unexpected virtual machine state result %T", raw)
	}

	return nil
}

func waitForVirtualMachineStableState(
	ctx context.Context,
	m *Meta,
	vmID string,
	timeout time.Duration,
) error {
	return waitForVirtualMachineState(
		ctx,
		m,
		vmID,
		[]core.VirtualMachineStateEnum{
			core.Allocated,
			core.Allocating,
			core.Resetting,
			core.Migrating,
			core.Transferring,
			core.Starting,
			core.Stopping,
			core.ShuttingDown,
		},
		[]core.VirtualMachineStateEnum{core.Started, core.Stopped},
		timeout,
	)
}

func virtualMachineExactStatePending(
	target core.VirtualMachineStateEnum,
) []core.VirtualMachineStateEnum {
	states := []core.VirtualMachineStateEnum{
		core.Allocated,
		core.Allocating,
		core.Migrating,
		core.Resetting,
		core.ShuttingDown,
		core.Started,
		core.Starting,
		core.Stopped,
		core.Stopping,
		core.Transferring,
	}
	pending := make([]core.VirtualMachineStateEnum, 0, len(states)-1)
	for _, state := range states {
		if state != target {
			pending = append(pending, state)
		}
	}

	return pending
}

func virtualMachineStateStrings(
	states []core.VirtualMachineStateEnum,
) []string {
	values := make([]string, len(states))
	for i, state := range states {
		values[i] = string(state)
	}

	return values
}

func virtualMachineStateWaitError(
	vmID string,
	from core.VirtualMachineStateEnum,
	target core.VirtualMachineStateEnum,
	err error,
) error {
	return fmt.Errorf(
		"error waiting for virtual machine %s to reach %s from %s: %w",
		vmID, target, from, err,
	)
}

// validateVirtualMachinePackageChange fails when a package change would
// downgrade a Virtual Machine that cannot be stopped by the same apply.
// Katapult requires the VM to be stopped before its vCPU count or memory can
// be reduced.
func validateVirtualMachinePackageChange(
	ctx context.Context,
	client virtualMachinePackageReader,
	vmID string,
	pkgRef string,
	poweredOn types.Bool,
) error {
	vm, err := virtualMachineForPackageValidation(ctx, client, vmID)
	if err != nil {
		return err
	}
	if vm == nil {
		return nil
	}
	if !vm.Package.IsSpecified() || vm.Package.IsNull() {
		return fmt.Errorf("virtual machine response is missing package details")
	}

	currentPkg, _ := vm.Package.Get()
	if virtualMachinePackageMatches(currentPkg, pkgRef) {
		return nil
	}
	// Every package change is safe while the VM is already stopped, so there
	// is no need to fetch the target package merely to classify the change.
	if vm.State != nil && *vm.State == core.Stopped {
		return nil
	}

	newPkg, err := virtualMachinePackageForValidation(ctx, client, pkgRef)
	if err != nil {
		return err
	}

	if currentPkg.CpuCores == nil || currentPkg.MemoryInGb == nil ||
		newPkg.CpuCores == nil || newPkg.MemoryInGb == nil {
		return fmt.Errorf("package response is missing vCPU or memory details")
	}

	if *newPkg.CpuCores < *currentPkg.CpuCores ||
		*newPkg.MemoryInGb < *currentPkg.MemoryInGb {
		if vm.State == nil {
			return fmt.Errorf("virtual machine response is missing state")
		}
		allowed, stateErr := virtualMachineDowngradeAllowed(
			*vm.State, poweredOn,
		)
		if stateErr != nil {
			return stateErr
		}
		if allowed {
			return nil
		}

		return fmt.Errorf(
			"cannot downgrade package unless the Virtual Machine is already "+
				"stopped or powered_on = false is explicitly configured in "+
				"the same plan: "+
				"current package has %d vCPU(s) and %dGB memory, new "+
				"package has %d vCPU(s) and %dGB memory. Apply the "+
				"downgrade with powered_on = false, then set it to true "+
				"in a later apply to start the Virtual Machine again",
			*currentPkg.CpuCores,
			*currentPkg.MemoryInGb,
			*newPkg.CpuCores,
			*newPkg.MemoryInGb,
		)
	}

	return nil
}

func virtualMachineDowngradeAllowed(
	state core.VirtualMachineStateEnum,
	poweredOn types.Bool,
) (bool, error) {
	switch state {
	case core.Stopped:
		return true, nil
	case core.Failed, core.Orphaned:
		return false, fmt.Errorf(
			"cannot downgrade package while Virtual Machine is in %s state",
			state,
		)
	case core.Started, core.Starting, core.Stopping, core.ShuttingDown,
		core.Allocated, core.Allocating, core.Resetting, core.Migrating,
		core.Transferring:
		return !poweredOn.IsNull() && !poweredOn.IsUnknown() &&
			!poweredOn.ValueBool(), nil
	default:
		return false, fmt.Errorf(
			"cannot downgrade package while Virtual Machine is in unsupported state %q",
			state,
		)
	}
}

func virtualMachineForPackageValidation(
	ctx context.Context,
	client virtualMachinePackageReader,
	vmID string,
) (*core.GetVirtualMachine200ResponseVirtualMachine, error) {
	vmRes, err := client.GetVirtualMachineWithResponse(
		ctx,
		&core.GetVirtualMachineParams{VirtualMachineId: &vmID},
	)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) ||
			(vmRes != nil && isErrNotFoundOrInTrash(err, vmRes.JSON406)) {
			return nil, nil
		}
		if vmRes != nil {
			err = genericAPIError(err, vmRes.Body)
		}

		return nil, fmt.Errorf(
			"failed to fetch virtual machine details: %w",
			err,
		)
	}
	if vmRes == nil || vmRes.JSON200 == nil {
		return nil, fmt.Errorf(
			"unexpected empty response fetching virtual machine",
		)
	}

	return &vmRes.JSON200.VirtualMachine, nil
}

func virtualMachinePackageForValidation(
	ctx context.Context,
	client virtualMachinePackageReader,
	pkgRef string,
) (*core.VirtualMachinePackage, error) {
	pkgRes, err := client.GetVirtualMachinePackageWithResponse(
		ctx,
		virtualMachinePackageParams(pkgRef),
	)
	if err != nil {
		if pkgRes != nil {
			err = genericAPIError(err, pkgRes.Body)
		}

		return nil, fmt.Errorf("failed to fetch new package details: %w", err)
	}
	if pkgRes == nil || pkgRes.JSON200 == nil {
		return nil, fmt.Errorf("unexpected empty response fetching new package")
	}

	return &pkgRes.JSON200.VirtualMachinePackage, nil
}

func changeVirtualMachinePackage(
	ctx context.Context,
	m *Meta,
	vmID string,
	pkgRef string,
	timeout time.Duration,
) error {
	vmRes, err := m.Core.GetVirtualMachineWithResponse(
		ctx,
		&core.GetVirtualMachineParams{VirtualMachineId: &vmID},
	)
	if err != nil {
		if vmRes != nil {
			err = genericAPIError(err, vmRes.Body)
		}

		return fmt.Errorf("failed to fetch virtual machine details: %w", err)
	}
	if vmRes == nil || vmRes.JSON200 == nil {
		return fmt.Errorf("unexpected empty response fetching virtual machine")
	}

	vm := vmRes.JSON200.VirtualMachine
	if vm.Package.IsSpecified() {
		currentPkg, getErr := vm.Package.Get()
		if getErr == nil && virtualMachinePackageMatches(currentPkg, pkgRef) {
			return nil
		}
	}

	changeRes, err := m.Core.PutVirtualMachinePackageWithResponse(
		ctx,
		core.PutVirtualMachinePackageJSONRequestBody{
			VirtualMachine:        core.VirtualMachineLookup{Id: &vmID},
			VirtualMachinePackage: virtualMachinePackageLookup(pkgRef),
		},
	)
	if err != nil {
		if changeRes != nil {
			err = genericAPIError(err, changeRes.Body)
		}

		return fmt.Errorf("failed to change virtual machine package: %w", err)
	}
	if changeRes == nil || changeRes.JSON200 == nil ||
		changeRes.JSON200.Task.Id == nil {
		return fmt.Errorf(
			"unexpected empty task response changing virtual machine package",
		)
	}

	return waitForTaskCompletion(
		ctx,
		m,
		timeout,
		*changeRes.JSON200.Task.Id,
	)
}

func virtualMachinePackageLookup(
	value string,
) core.VirtualMachinePackageLookup {
	if strings.HasPrefix(value, "vmpkg_") {
		return core.VirtualMachinePackageLookup{Id: &value}
	}

	return core.VirtualMachinePackageLookup{Permalink: &value}
}

func virtualMachinePackageParams(
	value string,
) *core.GetVirtualMachinePackageParams {
	if strings.HasPrefix(value, "vmpkg_") {
		return &core.GetVirtualMachinePackageParams{
			VirtualMachinePackageId: &value,
		}
	}

	return &core.GetVirtualMachinePackageParams{
		VirtualMachinePackagePermalink: &value,
	}
}

func virtualMachinePackageMatches(
	pkg core.VirtualMachinePackage,
	value string,
) bool {
	return (pkg.Id != nil && *pkg.Id == value) ||
		(pkg.Permalink != nil && *pkg.Permalink == value)
}

// normalizeVirtualMachinePackageForState returns the package value to store in
// state, preserving whichever format (ID or permalink) the user configured.
// This prevents perpetual diffs when config uses IDs but the API returns
// permalinks (or vice versa).
func normalizeVirtualMachinePackageForState(
	configured string,
	pkg core.VirtualMachinePackage,
) string {
	if strings.HasPrefix(configured, "vmpkg_") && pkg.Id != nil &&
		*pkg.Id != "" {
		return *pkg.Id
	}

	if pkg.Permalink != nil && *pkg.Permalink != "" {
		return *pkg.Permalink
	}

	if pkg.Id != nil {
		return *pkg.Id
	}

	return ""
}

func buildVMNetworkInterfaceList(
	ifaces []*core.GetVMNIVMNI200ResponseVirtualMachineNetworkInterface,
) (types.List, error) {
	niObjType := types.ObjectType{AttrTypes: vmNetworkInterfaceAttrTypes}

	elems := make([]attr.Value, 0, len(ifaces))
	for _, iface := range ifaces {
		niID := types.StringNull()
		if iface.Id != nil {
			niID = types.StringValue(*iface.Id)
		}

		netID := types.StringNull()
		if iface.Network.IsSpecified() {
			if net, err := iface.Network.Get(); err == nil &&
				net.Id != nil {
				netID = types.StringValue(*net.Id)
			}
		}

		vnetID := types.StringNull()
		if iface.VirtualNetwork.IsSpecified() {
			if vnet, err := iface.VirtualNetwork.Get(); err == nil &&
				vnet.Id != nil {
				vnetID = types.StringValue(*vnet.Id)
			}
		}

		macAddr := types.StringNull()
		if iface.MacAddress != nil {
			macAddr = types.StringValue(*iface.MacAddress)
		}

		ipAddrs := make([]attr.Value, 0)
		if iface.IpAddresses != nil {
			for _, ip := range *iface.IpAddresses {
				if ip.Address != nil {
					ipAddrs = append(
						ipAddrs, types.StringValue(*ip.Address),
					)
				}
			}
		}
		ipSet := types.SetValueMust(types.StringType, ipAddrs)

		obj, diags := types.ObjectValue(
			vmNetworkInterfaceAttrTypes,
			map[string]attr.Value{
				"id":                 niID,
				"network_id":         netID,
				"virtual_network_id": vnetID,
				"mac_address":        macAddr,
				"ip_addresses":       ipSet,
			},
		)
		if diags.HasError() {
			return types.ListNull(niObjType), fmt.Errorf(
				"error building network interface object: %s", diags,
			)
		}
		elems = append(elems, obj)
	}

	list, diags := types.ListValue(niObjType, elems)
	if diags.HasError() {
		return types.ListNull(niObjType), fmt.Errorf(
			"error building network interface list: %s", diags,
		)
	}

	return list, nil
}

func stringSetValueStrings(
	ctx context.Context,
	attributeName string,
	set basetypes.SetValue,
) ([]string, diag.Diagnostics) {
	var diags diag.Diagnostics
	if set.IsNull() {
		return []string{}, diags
	}

	values := []types.String{}
	diags.Append(set.ElementsAs(ctx, &values, true)...)
	if diags.HasError() {
		return nil, diags
	}

	strings := make([]string, 0, len(values))
	for _, value := range values {
		if value.IsNull() {
			continue
		}
		if value.IsUnknown() {
			diags.AddError(
				"Value Conversion Error",
				fmt.Sprintf(
					"%s contains unknown values during update",
					attributeName,
				),
			)
			return nil, diags
		}
		strings = append(strings, value.ValueString())
	}

	return strings, diags
}

// fetchAllVMNetworkInterfaces returns all network interfaces for a VM,
// fetching full interface details and deduplicating by ID.
func fetchAllVMNetworkInterfaces(
	ctx context.Context,
	m *Meta,
	vmID string,
) ([]*core.GetVMNIVMNI200ResponseVirtualMachineNetworkInterface, error) {
	results := make(
		map[string]*core.GetVMNIVMNI200ResponseVirtualMachineNetworkInterface,
	)

	totalPages := 1
	for page := 1; page <= totalPages; page++ {
		params := &core.GetVirtualMachineNetworkInterfacesParams{
			VirtualMachineId: &vmID,
		}
		if page > 1 {
			p := page
			params.Page = &p
		}

		resp, err := m.Core.GetVirtualMachineNetworkInterfacesWithResponse(
			ctx,
			params,
		)
		if err != nil {
			if resp != nil {
				return nil, genericAPIError(err, resp.Body)
			}
			return nil, err
		}

		if resp.JSON200 == nil {
			return nil, fmt.Errorf("unexpected empty response")
		}

		body := resp.JSON200
		if body.Pagination.TotalPages.IsSpecified() {
			n, _ := body.Pagination.TotalPages.Get()
			totalPages = n
		}

		for i := range body.VirtualMachineNetworkInterfaces {
			iface := body.VirtualMachineNetworkInterfaces[i]
			if iface.Id == nil {
				continue
			}
			vmni, errGet := getVMNetworkInterface(ctx, m, *iface.Id)
			if errGet != nil {
				return nil, errGet
			}
			if vmni.Id != nil {
				results[*vmni.Id] = vmni
			}
		}
	}

	ifaces := make(
		[]*core.GetVMNIVMNI200ResponseVirtualMachineNetworkInterface,
		0, len(results),
	)
	for _, iface := range results {
		ifaces = append(ifaces, iface)
	}

	sort.Slice(ifaces, func(i, j int) bool {
		return *ifaces[i].Id < *ifaces[j].Id
	})

	return ifaces, nil
}

func getVMNetworkInterface(
	ctx context.Context,
	m *Meta,
	ifaceID string,
) (*core.GetVMNIVMNI200ResponseVirtualMachineNetworkInterface, error) {
	res, err := m.Core.GetVMNIVMNIWithResponse(ctx,
		&core.GetVMNIVMNIParams{
			VirtualMachineNetworkInterfaceId: &ifaceID,
		},
	)
	if err != nil {
		if res != nil {
			if res.StatusCode() == http.StatusNotFound {
				return nil, core.ErrNotFound
			}
			return nil, genericAPIError(err, res.Body)
		}
		return nil, err
	}

	if res.JSON200 == nil {
		return nil, fmt.Errorf("unexpected empty response")
	}

	return &res.JSON200.VirtualMachineNetworkInterface, nil
}

func addVirtualNetworkToVM(
	ctx context.Context,
	m *Meta,
	vmID, vnetID, speedProfilePermalink string,
	timeout time.Duration,
) error {
	req := core.PostVirtualMachineNetworkInterfacesJSONRequestBody{
		VirtualMachine: core.VirtualMachineLookup{Id: &vmID},
		VirtualNetwork: &core.VirtualNetworkLookup{
			Id: &vnetID,
		},
	}
	if speedProfilePermalink != "" {
		req.SpeedProfile = core.NetworkSpeedProfileLookup{
			Permalink: &speedProfilePermalink,
		}
	}

	createResp, err := m.Core.
		PostVirtualMachineNetworkInterfacesWithResponse(ctx,
			req,
		)
	if err != nil {
		if createResp != nil {
			return genericAPIError(err, createResp.Body)
		}
		return err
	}

	if createResp.JSON200 == nil ||
		createResp.JSON200.VirtualMachineNetworkInterface.Id == nil {
		return fmt.Errorf("unexpected empty response")
	}

	ifaceID := *createResp.JSON200.VirtualMachineNetworkInterface.Id

	return attachVMNetworkInterface(ctx, m, ifaceID, timeout)
}

func attachVMNetworkInterface(
	ctx context.Context,
	m *Meta,
	ifaceID string,
	timeout time.Duration,
) error {
	attachResp, err := m.Core.
		PostVirtualMachineNetworkInterfaceAttachWithResponse(ctx,
			core.PostVirtualMachineNetworkInterfaceAttachJSONRequestBody{
				VirtualMachineNetworkInterface: core.
					VirtualMachineNetworkInterfaceLookup{
					Id: &ifaceID,
				},
			},
		)
	if err != nil {
		if attachResp != nil {
			return genericAPIError(err, attachResp.Body)
		}
		return err
	}

	if attachResp.JSON200 == nil || attachResp.JSON200.Task.Id == nil {
		return fmt.Errorf("unexpected empty response")
	}

	return waitForTaskCompletion(
		ctx, m, timeout, *attachResp.JSON200.Task.Id,
	)
}

func removeVMNetworkInterface(
	ctx context.Context,
	m *Meta,
	ifaceID string,
	timeout time.Duration,
) error {
	iface, err := getVMNetworkInterface(ctx, m, ifaceID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil
		}
		return err
	}

	if iface.State != nil && *iface.State == "attached" {
		err = detachVMNetworkInterface(ctx, m, ifaceID, timeout)
		if err != nil {
			return err
		}
	}

	delResp, err := m.Core.
		DeleteVirtualMachineNetworkInterfaceWithResponse(ctx,
			core.DeleteVirtualMachineNetworkInterfaceJSONRequestBody{
				VirtualMachineNetworkInterface: core.
					VirtualMachineNetworkInterfaceLookup{
					Id: &ifaceID,
				},
			},
		)
	if err != nil {
		if delResp != nil {
			if delResp.StatusCode() == http.StatusNotFound {
				return nil
			}
			return genericAPIError(err, delResp.Body)
		}
		return err
	}

	return nil
}

func detachVMNetworkInterface(
	ctx context.Context,
	m *Meta,
	ifaceID string,
	timeout time.Duration,
) error {
	detachResp, err := m.Core.
		PostVirtualMachineNetworkInterfaceDetachWithResponse(ctx,
			core.PostVirtualMachineNetworkInterfaceDetachJSONRequestBody{
				VirtualMachineNetworkInterface: core.
					VirtualMachineNetworkInterfaceLookup{
					Id: &ifaceID,
				},
			},
		)
	if err != nil {
		if detachResp != nil {
			if detachResp.StatusCode() == http.StatusNotFound {
				return nil
			}
			apiErr := parseGenericAPIError(detachResp.Body)
			if apiErr != nil &&
				apiErr.Code ==
					"virtual_machine_network_interface_not_attached" {
				return nil
			}
			return genericAPIError(err, detachResp.Body)
		}
		return err
	}

	if detachResp.JSON200 == nil || detachResp.JSON200.Task.Id == nil {
		return fmt.Errorf("unexpected empty response")
	}

	return waitForTaskCompletion(
		ctx, m, timeout, *detachResp.JSON200.Task.Id,
	)
}

func allocateIPsToVM(
	ctx context.Context,
	m *Meta,
	vmID string,
	ipIDs []string,
) error {
	if len(ipIDs) == 0 {
		return nil
	}

	ifaces, err := fetchAllVMNetworkInterfaces(ctx, m, vmID)
	if err != nil {
		return err
	}

	for _, ipID := range ipIDs {
		id := ipID
		ipRes, err := m.Core.GetIpAddressWithResponse(ctx,
			&core.GetIpAddressParams{IpAddressId: &id})
		if err != nil {
			if ipRes != nil {
				err = genericAPIError(err, ipRes.Body)
			}
			return err
		}

		if ipRes.JSON200 == nil {
			return fmt.Errorf("unexpected empty response fetching IP")
		}
		ip := ipRes.JSON200.IpAddress
		if ip.Network == nil || ip.Network.Id == nil {
			return fmt.Errorf(
				"could not determine network of IP: %s", ipID,
			)
		}
		networkID := *ip.Network.Id

		var vmnetID string
		for _, iface := range ifaces {
			if !iface.Network.IsSpecified() || iface.Id == nil {
				continue
			}
			net, err2 := iface.Network.Get()
			if err2 != nil || net.Id == nil {
				continue
			}
			if *net.Id == networkID {
				vmnetID = *iface.Id
				break
			}
		}

		if vmnetID == "" {
			return fmt.Errorf(
				"no usable network interface found for IP: %s", ipID,
			)
		}

		requestBody := core.PostVirtualMachineNetworkInterfaceAllocateIpJSONRequestBody{
			IpAddress: core.IPAddressLookup{Id: &id},
			VirtualMachineNetworkInterface: core.
				VirtualMachineNetworkInterfaceLookup{
				Id: &vmnetID,
			},
		}
		resp, err := m.Core.
			PostVirtualMachineNetworkInterfaceAllocateIpWithResponse(
				ctx, requestBody,
			)
		if err != nil {
			if resp != nil {
				return genericAPIError(err, resp.Body)
			}
			return err
		}
	}

	return nil
}

func updateVMNetworkSpeedProfile(
	ctx context.Context,
	m *Meta,
	vmID, permalink string,
	timeout time.Duration,
) error {
	if permalink == "" {
		return nil
	}

	ifaces, err := fetchAllVMNetworkInterfaces(ctx, m, vmID)
	if err != nil {
		return err
	}

	for _, iface := range ifaces {
		if iface.Id == nil {
			continue
		}
		ifaceID := *iface.Id

		requestBody := core.PatchVirtualMachineNetworkInterfaceUpdateSpeedProfileJSONRequestBody{
			VirtualMachineNetworkInterface: core.
				VirtualMachineNetworkInterfaceLookup{
				Id: &ifaceID,
			},
			SpeedProfile: core.NetworkSpeedProfileLookup{
				Permalink: &permalink,
			},
		}
		res, err := m.Core.
			PatchVirtualMachineNetworkInterfaceUpdateSpeedProfileWithResponse(
				ctx, requestBody,
			)
		if err != nil {
			if res != nil {
				if res.JSON422 != nil && res.JSON422.Code != nil &&
					*res.JSON422.Code ==
						core.SpeedProfileAlreadyAssigned {
					continue
				}
				return genericAPIError(err, res.Body)
			}
			return err
		}

		if res.JSON200 == nil || res.JSON200.Task.Id == nil {
			return fmt.Errorf("unexpected empty response")
		}

		if err := waitForTaskCompletion(
			ctx, m, timeout, *res.JSON200.Task.Id,
		); err != nil {
			return err
		}
	}

	return nil
}

func waitForVMToStop(
	ctx context.Context,
	m *Meta,
	vmID string,
	timeout time.Duration,
) error {
	err := waitForVirtualMachineState(
		ctx,
		m,
		vmID,
		[]core.VirtualMachineStateEnum{
			core.Started,
			core.Stopping,
			core.ShuttingDown,
		},
		[]core.VirtualMachineStateEnum{core.Stopped},
		timeout,
	)

	return err
}

func addVMUniqueHostnameSuffix(
	ctx context.Context,
	m *Meta,
	vmID string,
	currentHostname *string,
) (string, error) {
	id, err := rands.Alphanumeric(12)
	if err != nil {
		return "", err
	}

	hostname := ""
	if currentHostname != nil {
		hostname = *currentHostname
	}

	suffix := "-" + id
	if len(hostname)+len(suffix) > 63 {
		hostname = hostname[:63-len(suffix)]
	}
	hostname += suffix

	patchRes, err := m.Core.PatchVirtualMachineWithResponse(ctx,
		core.PatchVirtualMachineJSONRequestBody{
			VirtualMachine: core.VirtualMachineLookup{Id: &vmID},
			Properties: core.VirtualMachineArguments{
				Hostname: &hostname,
			},
		})
	if err != nil {
		if patchRes != nil {
			err = genericAPIError(err, patchRes.Body)
		}
		return "", err
	}

	return hostname, nil
}
