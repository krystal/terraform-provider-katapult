package v6provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
)

type (
	VirtualMachineDataSource struct {
		M *Meta
	}

	VirtualMachineDataSourceModel struct {
		ID                  types.String `tfsdk:"id"`
		Name                types.String `tfsdk:"name"`
		Hostname            types.String `tfsdk:"hostname"`
		Description         types.String `tfsdk:"description"`
		FQDN                types.String `tfsdk:"fqdn"`
		State               types.String `tfsdk:"state"`
		Package             types.String `tfsdk:"package"`
		IPAddressIDs        types.Set    `tfsdk:"ip_address_ids"`
		IPAddresses         types.Set    `tfsdk:"ip_addresses"`
		VirtualNetworkIDs   types.Set    `tfsdk:"virtual_network_ids"`
		NetworkSpeedProfile types.String `tfsdk:"network_speed_profile"`
		NetworkInterfaces   types.List   `tfsdk:"network_interfaces"`
		Tags                types.Set    `tfsdk:"tags"`
		GroupID             types.String `tfsdk:"group_id"`
	}
)

func (d *VirtualMachineDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_virtual_machine"
}

func (d *VirtualMachineDataSource) Configure(
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

	d.M = meta
}

func (d *VirtualMachineDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Retrieve details of an existing Virtual Machine.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "The unique identifier of the " +
					"Virtual Machine.",
				Validators: []validator.String{
					stringvalidator.AtLeastOneOf(
						path.MatchRoot("id"),
						path.MatchRoot("fqdn"),
					),
				},
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The name of the Virtual Machine.",
			},
			"hostname": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The hostname of the Virtual Machine.",
			},
			"description": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "A description for the Virtual Machine.",
			},
			"fqdn": schema.StringAttribute{
				Optional: true,
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
				Computed: true,
				MarkdownDescription: "Permalink or ID of the Virtual " +
					"Machine Package.",
			},
			"ip_address_ids": schema.SetAttribute{
				Computed:    true,
				ElementType: types.StringType,
				MarkdownDescription: "Set of IP address IDs allocated to " +
					"the Virtual Machine.",
			},
			"ip_addresses": schema.SetAttribute{
				Computed:    true,
				ElementType: types.StringType,
				MarkdownDescription: "Set of IP addresses allocated to " +
					"the Virtual Machine.",
			},
			"virtual_network_ids": schema.SetAttribute{
				Computed:    true,
				ElementType: types.StringType,
				MarkdownDescription: "Set of Virtual Network IDs attached " +
					"to the Virtual Machine.",
			},
			"network_speed_profile": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "Permalink of the Network Speed " +
					"Profile applied to all network interfaces.",
			},
			"network_interfaces": schema.ListNestedAttribute{
				Computed: true,
				MarkdownDescription: "Network interface details for " +
					"the Virtual Machine.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The ID of the network interface.",
						},
						"network_id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The ID of the network the interface is on.",
						},
						"virtual_network_id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The ID of the virtual network the interface is on.",
						},
						"mac_address": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The MAC address of the interface.",
						},
						"ip_addresses": schema.SetAttribute{
							Computed:    true,
							ElementType: types.StringType,
							MarkdownDescription: "The IP addresses allocated " +
								"to the interface.",
						},
					},
				},
			},
			"tags": schema.SetAttribute{
				Computed:    true,
				ElementType: types.StringType,
				MarkdownDescription: "Set of tag names assigned to the " +
					"Virtual Machine.",
			},
			"group_id": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "The ID of the Virtual Machine Group " +
					"this Virtual Machine belongs to.",
			},
		},
	}
}

//nolint:gocyclo
func (d *VirtualMachineDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var data VirtualMachineDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var params core.GetVirtualMachineParams
	switch {
	case !data.ID.IsNull() && !data.ID.IsUnknown() && data.ID.ValueString() != "":
		params.VirtualMachineId = data.ID.ValueStringPointer()
	case !data.FQDN.IsNull() && !data.FQDN.IsUnknown() && data.FQDN.ValueString() != "":
		params.VirtualMachineFqdn = data.FQDN.ValueStringPointer()
	default:
		resp.Diagnostics.AddError(
			"Read Error",
			`one of "id", "fqdn" must be specified`,
		)
		return
	}

	vmRes, err := d.M.Core.GetVirtualMachineWithResponse(ctx, &params)
	if err != nil {
		if vmRes != nil {
			err = genericAPIError(err, vmRes.Body)
		}
		resp.Diagnostics.AddError("Read Error", err.Error())
		return
	}

	vm := vmRes.JSON200.VirtualMachine
	vmID := ""
	if vm.Id != nil {
		vmID = *vm.Id
	}

	data.ID = types.StringPointerValue(vm.Id)
	data.Name = types.StringPointerValue(vm.Name)
	data.Hostname = types.StringPointerValue(vm.Hostname)
	data.FQDN = types.StringPointerValue(vm.Fqdn)

	if vm.State != nil {
		data.State = types.StringValue(string(*vm.State))
	}

	if desc, err2 := vm.Description.Get(); err2 == nil {
		data.Description = types.StringValue(desc)
	} else {
		data.Description = types.StringValue("")
	}

	if vm.Package.IsSpecified() {
		if pkg, err2 := vm.Package.Get(); err2 == nil {
			if pkg.Permalink != nil && *pkg.Permalink != "" {
				data.Package = types.StringPointerValue(pkg.Permalink)
			} else if pkg.Id != nil {
				data.Package = types.StringPointerValue(pkg.Id)
			}
		}
	}

	if vm.Group.IsSpecified() {
		if grp, err2 := vm.Group.Get(); err2 == nil && grp.Id != nil {
			data.GroupID = types.StringPointerValue(grp.Id)
		} else {
			data.GroupID = types.StringNull()
		}
	} else {
		data.GroupID = types.StringNull()
	}

	if vm.IpAddresses != nil {
		ipIDs := make([]attr.Value, 0, len(*vm.IpAddresses))
		ipAddrs := make([]attr.Value, 0, len(*vm.IpAddresses))
		for _, ip := range *vm.IpAddresses {
			if ip.Id != nil {
				ipIDs = append(ipIDs, types.StringValue(*ip.Id))
			}
			if ip.Address != nil {
				ipAddrs = append(ipAddrs, types.StringValue(*ip.Address))
			}
		}
		data.IPAddressIDs = types.SetValueMust(types.StringType, ipIDs)
		data.IPAddresses = types.SetValueMust(types.StringType, ipAddrs)
	} else {
		data.IPAddressIDs = types.SetValueMust(types.StringType, []attr.Value{})
		data.IPAddresses = types.SetValueMust(types.StringType, []attr.Value{})
	}

	if vm.TagNames != nil {
		tagVals := make([]attr.Value, 0, len(*vm.TagNames))
		for _, t := range *vm.TagNames {
			tagVals = append(tagVals, types.StringValue(t))
		}
		data.Tags = types.SetValueMust(types.StringType, tagVals)
	} else {
		data.Tags = types.SetValueMust(types.StringType, []attr.Value{})
	}

	ifaces, err := fetchAllVMNetworkInterfaces(ctx, d.M, vmID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Read Error",
			fmt.Sprintf("error fetching network interfaces: %s", err),
		)
		return
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
	data.VirtualNetworkIDs = types.SetValueMust(types.StringType, vnetIDs)

	if len(ifaces) > 0 && ifaces[0].SpeedProfile != nil &&
		ifaces[0].SpeedProfile.Permalink != nil {
		data.NetworkSpeedProfile = types.StringValue(
			*ifaces[0].SpeedProfile.Permalink,
		)
	} else {
		data.NetworkSpeedProfile = types.StringValue("")
	}

	niList, err := buildVMNetworkInterfaceList(ifaces)
	if err != nil {
		resp.Diagnostics.AddError(
			"Read Error",
			fmt.Sprintf("error building network interface list: %s", err),
		)
		return
	}
	data.NetworkInterfaces = niList

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
