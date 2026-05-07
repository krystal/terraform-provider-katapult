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

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
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
		ID                  types.String `tfsdk:"id"`
		Name                types.String `tfsdk:"name"`
		Hostname            types.String `tfsdk:"hostname"`
		Description         types.String `tfsdk:"description"`
		FQDN                types.String `tfsdk:"fqdn"`
		State               types.String `tfsdk:"state"`
		Package             types.String `tfsdk:"package"`
		DiskTemplate        types.String `tfsdk:"disk_template"`
		DiskTemplateOptions types.Map    `tfsdk:"disk_template_options"`
		Disk                types.List   `tfsdk:"disk"`
		IPAddressIDs        types.Set    `tfsdk:"ip_address_ids"`
		IPAddresses         types.Set    `tfsdk:"ip_addresses"`
		VirtualNetworkIDs   types.Set    `tfsdk:"virtual_network_ids"`
		NetworkSpeedProfile types.String `tfsdk:"network_speed_profile"`
		NetworkInterfaces   types.List   `tfsdk:"network_interfaces"`
		Tags                types.Set    `tfsdk:"tags"`
		GroupID             types.String `tfsdk:"group_id"`
	}

	VirtualMachineDiskModel struct {
		Name types.String `tfsdk:"name"`
		Size types.Int64  `tfsdk:"size"`
	}
)

var vmNetworkInterfaceAttrTypes = map[string]attr.Type{
	"id":                 types.StringType,
	"network_id":         types.StringType,
	"virtual_network_id": types.StringType,
	"mac_address":        types.StringType,
	"ip_addresses":       types.SetType{ElemType: types.StringType},
}

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

func (r *VirtualMachineResource) Schema( //nolint:funlen
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Virtual Machine in Katapult.\n\n" +
			"~> **Warning:** Deleting a virtual machine resource will by " +
			"default purge the VM from Katapult's trash, permanently " +
			"deleting it. Set `skip_trash_object_purge` on the " +
			"provider to keep it in the trash instead.",
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
				MarkdownDescription: "A description for the " +
					"Virtual Machine.",
			},
			"fqdn": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "The fully-qualified domain name of " +
					"the Virtual Machine.",
			},
			"state": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "The current state of the " +
					"Virtual Machine.",
			},
			"package": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Permalink or ID of the Virtual " +
					"Machine Package to use.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"disk_template": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Permalink or ID of the Disk " +
					"Template to use.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"disk_template_options": schema.MapAttribute{
				Optional:    true,
				ElementType: types.StringType,
				MarkdownDescription: "Options to pass to the Disk " +
					"Template during creation.",
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.RequiresReplace(),
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
				MarkdownDescription: "The ID of the Virtual Machine Group " +
					"to assign this Virtual Machine to.",
			},
		},
		Blocks: map[string]schema.Block{
			"disk": schema.ListNestedBlock{
				MarkdownDescription: "One or more disks with custom sizes " +
					"to create and attach during creation. The first " +
					"disk is the boot disk. If omitted, a single disk " +
					"is created from the chosen package.",
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Optional: true,
							MarkdownDescription: "Name of the disk. " +
								"Defaults to \"System Disk\" for " +
								"the first disk.",
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.RequiresReplace(),
							},
						},
						"size": schema.Int64Attribute{
							Required:            true,
							MarkdownDescription: "Size of the disk in GB.",
						},
					},
				},
			},
		},
	}
}

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

	timeout := 20 * time.Minute

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
	planTags, diags := stringSetValueStrings(ctx, targetTags)
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
	ipIDs, diags := stringSetValueStrings(ctx, targetIPIDs)
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
			resp.Diagnostics.AddError("Create Error", "unexpected empty response fetching IP")
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
	vnetIDs, diags := stringSetValueStrings(ctx, targetVnetIDs)
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
		resp.Diagnostics.AddError("Create Error", "unexpected empty response from build")
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
				return nil, "", fmt.Errorf("unexpected empty response polling build")
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
		Delay:                     2 * time.Second,
		MinTimeout:                5 * time.Second,
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
				return nil, "", fmt.Errorf("unexpected empty response polling VM state")
			}
			v := res.JSON200.VirtualMachine
			if v.State == nil {
				return v, "", fmt.Errorf("vm state is nil")
			}
			return v, string(*v.State), nil
		},
		Timeout:                   timeout,
		Delay:                     2 * time.Second,
		MinTimeout:                5 * time.Second,
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

	if err := r.vmRead(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Read Error", err.Error())
		return
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

	timeout := 10 * time.Minute
	vmID := state.ID.ValueString()

	args := core.VirtualMachineArguments{}

	if !plan.Name.Equal(state.Name) {
		args.Name = plan.Name.ValueStringPointer()
	}
	if !plan.Hostname.Equal(state.Hostname) {
		args.Hostname = plan.Hostname.ValueStringPointer()
	}
	if !plan.Description.Equal(state.Description) {
		args.Description = plan.Description.ValueStringPointer()
	}
	targetTags := plan.Tags
	if targetTags.IsUnknown() {
		resp.Diagnostics.Append(
			req.Config.GetAttribute(ctx, path.Root("tags"), &targetTags)...,
		)
	}
	if !targetTags.IsUnknown() && !targetTags.Equal(state.Tags) {
		tags, diags := stringSetValueStrings(ctx, targetTags)
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
			core.VirtualMachineGroupLookup{Id: plan.GroupID.ValueStringPointer()},
		)
		rg := json.RawMessage(groupBytes)
		props.Group = &rg
	}

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

		targetIDs, diags := stringSetValueStrings(ctx, targetIPIDs)
		resp.Diagnostics.Append(diags...)

		stateIDs, diags := stringSetValueStrings(ctx, state.IPAddressIDs)
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
		targetIDs, diags := stringSetValueStrings(ctx, targetVnetIDs)
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
			if *iface.State == "attached" {
				attachedVnetIDs = append(
					attachedVnetIDs, *vnet.Id,
				)
			} else if *iface.State == "detached" {
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

	if !plan.NetworkSpeedProfile.Equal(state.NetworkSpeedProfile) {
		permalink := plan.NetworkSpeedProfile.ValueString()
		if e := updateVMNetworkSpeedProfile(
			ctx, r.M, vmID, permalink, timeout,
		); e != nil {
			resp.Diagnostics.AddError("Update Error", e.Error())
			return
		}
	}

	if err := r.vmRead(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Read Error", err.Error())
		return
	}
	if !plan.NetworkSpeedProfile.Equal(state.NetworkSpeedProfile) &&
		plan.NetworkSpeedProfile.ValueString() == "" {
		plan.NetworkSpeedProfile = types.StringNull()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

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

	timeout := 10 * time.Minute
	vmID := state.ID.ValueString()

	vmRes, err := r.M.Core.GetVirtualMachineWithResponse(ctx,
		&core.GetVirtualMachineParams{VirtualMachineId: &vmID})
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return
		}
		if vmRes != nil {
			err = genericAPIError(err, vmRes.Body)
		}
		resp.Diagnostics.AddError("Delete Error", err.Error())
		return
	}
	if vmRes.JSON200 == nil {
		resp.Diagnostics.AddError("Delete Error", "unexpected empty response fetching VM")
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
	if err != nil {
		if delRes == nil {
			resp.Diagnostics.AddError(
				"Delete Error",
				fmt.Sprintf("failed to delete VM: %s", err),
			)
			return
		}

		err = genericAPIError(err, delRes.Body)
		if !isErrNotFoundOrInTrash(err, delRes.JSON406) {
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

	if !r.M.SkipTrashObjectPurge &&
		delRes != nil && delRes.JSON200 != nil {
		trashObj := delRes.JSON200.TrashObject
		if e := purgeTrashObject(
			ctx, r.M, timeout, trashObj,
		); e != nil && !isErrNotFoundOrInTrash(e, nil) {
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
}

//nolint:gocyclo
func (r *VirtualMachineResource) vmRead(
	ctx context.Context,
	model *VirtualMachineResourceModel,
) error {
	vmID := model.ID.ValueString()

	vmRes, err := r.M.Core.GetVirtualMachineWithResponse(ctx,
		&core.GetVirtualMachineParams{VirtualMachineId: &vmID})
	if err != nil {
		if vmRes != nil {
			err = genericAPIError(err, vmRes.Body)
		}
		return err
	}
	if vmRes.JSON200 == nil {
		return fmt.Errorf("unexpected empty response fetching VM")
	}
	vm := vmRes.JSON200.VirtualMachine

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
		model.State = types.StringValue(string(*vm.State))
	}

	if desc, err2 := vm.Description.Get(); err2 == nil && desc != "" {
		model.Description = types.StringValue(desc)
	} else {
		model.Description = types.StringNull()
	}

	if vm.Package.IsSpecified() {
		if pkg, err2 := vm.Package.Get(); err2 == nil {
			if pkg.Permalink != nil && *pkg.Permalink != "" {
				model.Package = types.StringPointerValue(pkg.Permalink)
			} else if pkg.Id != nil {
				model.Package = types.StringPointerValue(pkg.Id)
			}
		}
	}

	if vm.Group.IsSpecified() {
		if grp, err2 := vm.Group.Get(); err2 == nil && grp.Id != nil {
			model.GroupID = types.StringPointerValue(grp.Id)
		} else {
			model.GroupID = types.StringNull()
		}
	} else {
		model.GroupID = types.StringNull()
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
				"ip_address_ids contains unknown values during update",
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

	totalPages := 2
	for page := 1; page <= totalPages; page++ {
		resp, err := m.Core.GetVirtualMachineNetworkInterfacesWithResponse(
			ctx,
			&core.GetVirtualMachineNetworkInterfacesParams{
				VirtualMachineId: &vmID,
				Page:             &page,
			},
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

		resp, err := m.Core.
			PostVirtualMachineNetworkInterfaceAllocateIpWithResponse(
				ctx,
				core.PostVirtualMachineNetworkInterfaceAllocateIpJSONRequestBody{
					IpAddress: core.IPAddressLookup{Id: &id},
					VirtualMachineNetworkInterface: core.
						VirtualMachineNetworkInterfaceLookup{
						Id: &vmnetID,
					},
				},
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

		res, err := m.Core.
			PatchVirtualMachineNetworkInterfaceUpdateSpeedProfileWithResponse(
				ctx,
				core.PatchVirtualMachineNetworkInterfaceUpdateSpeedProfileJSONRequestBody{
					VirtualMachineNetworkInterface: core.
						VirtualMachineNetworkInterfaceLookup{
						Id: &ifaceID,
					},
					SpeedProfile: core.NetworkSpeedProfileLookup{
						Permalink: &permalink,
					},
				},
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
	waiter := &retry.StateChangeConf{
		Pending: []string{
			string(core.Started),
			string(core.Stopping),
			string(core.ShuttingDown),
		},
		Target: []string{
			string(core.Stopped),
		},
		Refresh: func() (interface{}, string, error) {
			res, e := m.Core.GetVirtualMachineWithResponse(ctx,
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
				return nil, "", fmt.Errorf("unexpected empty response polling VM state")
			}
			v := res.JSON200.VirtualMachine
			if v.State == nil {
				return v, "", fmt.Errorf("vm state is nil")
			}
			return v, string(*v.State), nil
		},
		Timeout:                   timeout,
		Delay:                     1 * time.Second,
		MinTimeout:                5 * time.Second,
		ContinuousTargetOccurence: 1,
	}

	_, err := waiter.WaitForStateContext(ctx)

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
