package v6provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
	"golang.org/x/sync/errgroup"
)

// diskFetchConcurrency caps the number of in-flight GetDisk calls when the
// data source resolves a VM's disks. The list endpoint
// (GetVirtualMachineDisks) does not return storage_speed, bus_type, or
// io_profile, so each attachment requires a follow-up GetDisk; we run those
// concurrently to keep wall-clock low without overwhelming the API. Five is a
// reasonable ceiling because most VMs only have a small number of attached
// disks; anything materially above that is unusual, so wider fan-out adds
// little benefit in practice.
const diskFetchConcurrency = 5

type (
	VirtualMachineDisksDataSource struct {
		M *Meta
	}

	VirtualMachineDisksDataSourceModel struct {
		VirtualMachineID types.String `tfsdk:"virtual_machine_id"`
		Disks            types.List   `tfsdk:"disks"`
	}
)

var vmDiskAttrTypes = map[string]attr.Type{
	"id":            types.StringType,
	"name":          types.StringType,
	"size_in_gb":    types.Int64Type,
	"storage_speed": types.StringType,
	"bus_type":      types.StringType,
	"io_profile_id": types.StringType,
	"wwn":           types.StringType,
	"state":         types.StringType,
	"boot":          types.BoolType,
}

func (d *VirtualMachineDisksDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_virtual_machine_disks"
}

func (d *VirtualMachineDisksDataSource) Configure(
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

func (d *VirtualMachineDisksDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists all disks (boot and additional) " +
			"attached to a Virtual Machine.",
		Attributes: map[string]schema.Attribute{
			"virtual_machine_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The ID of the Virtual Machine.",
			},
			"disks": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The list of disks attached to the VM.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The unique identifier of the disk.",
						},
						"name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The name of the disk.",
						},
						"size_in_gb": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "Size of the disk in GB.",
						},
						"storage_speed": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Storage speed of the disk.",
						},
						"bus_type": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Bus type of the disk.",
						},
						"io_profile_id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The IO profile ID of the disk.",
						},
						"wwn": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "World Wide Name of the disk.",
						},
						"state": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Current state of the disk.",
						},
						"boot": schema.BoolAttribute{
							Computed:            true,
							MarkdownDescription: "Whether this is the boot disk.",
						},
					},
				},
			},
		},
	}
}

func (d *VirtualMachineDisksDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var state VirtualMachineDisksDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vmID := state.VirtualMachineID.ValueString()

	attachments, err := fetchAllVMDisks(ctx, d.M, vmID)
	if err != nil {
		resp.Diagnostics.AddError("Read Error", err.Error())
		return
	}

	disks, err := fetchDiskDetailsForAttachments(ctx, d.M, attachments)
	if err != nil {
		resp.Diagnostics.AddError("Read Error", err.Error())
		return
	}

	diskObjType := types.ObjectType{AttrTypes: vmDiskAttrTypes}
	elems := make([]attr.Value, 0, len(disks))
	for i, disk := range disks {
		obj, diags := buildDiskAttrObject(disk, attachments[i].Boot)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		elems = append(elems, obj)
	}

	diskList, diags := types.ListValue(diskObjType, elems)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state.Disks = diskList
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// fetchDiskDetailsForAttachments resolves each attachment's full disk record
// via GetDisk, capped at diskFetchConcurrency in-flight requests. Returns a
// slice of disks parallel to the input attachments, with nil entries for
// attachments missing a disk ID.
func fetchDiskDetailsForAttachments(
	ctx context.Context,
	m *Meta,
	attachments []core.GetVirtualMachineDisks200ResponseDisks,
) ([]*core.GetDisk200ResponseDisk, error) {
	disks := make([]*core.GetDisk200ResponseDisk, len(attachments))

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(diskFetchConcurrency)

	for i, attachment := range attachments {
		if attachment.Disk == nil || attachment.Disk.Id == nil {
			continue
		}
		idx, diskID := i, *attachment.Disk.Id
		g.Go(func() error {
			res, err := m.Core.GetDiskWithResponse(gctx,
				&core.GetDiskParams{DiskId: &diskID})
			if err != nil {
				if res != nil {
					err = genericAPIError(err, res.Body)
				}
				return fmt.Errorf("fetching disk %s: %w", diskID, err)
			}
			if res.JSON200 == nil {
				return fmt.Errorf(
					"unexpected empty response fetching disk %s", diskID,
				)
			}
			disks[idx] = &res.JSON200.Disk
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}
	return disks, nil
}

// buildDiskAttrObject converts a fetched disk plus its attachment-level boot
// flag into the data source's nested-object value.
func buildDiskAttrObject(
	disk *core.GetDisk200ResponseDisk,
	bootFlag *bool,
) (attr.Value, diag.Diagnostics) {
	if disk == nil {
		return types.ObjectNull(vmDiskAttrTypes), nil
	}

	id := types.StringNull()
	if disk.Id != nil {
		id = types.StringValue(*disk.Id)
	}
	name := types.StringNull()
	if disk.Name != nil {
		name = types.StringValue(*disk.Name)
	}
	sizeInGB := types.Int64Null()
	if disk.SizeInGb != nil {
		sizeInGB = types.Int64Value(int64(*disk.SizeInGb))
	}
	storageSpeed := types.StringNull()
	if disk.StorageSpeed != nil {
		storageSpeed = types.StringValue(string(*disk.StorageSpeed))
	}
	busType := types.StringNull()
	if disk.BusType.IsSpecified() {
		if bt, e := disk.BusType.Get(); e == nil {
			busType = types.StringValue(string(bt))
		}
	}
	ioProfileID := types.StringNull()
	if disk.IoProfile.IsSpecified() {
		if iop, e := disk.IoProfile.Get(); e == nil && iop.Id != nil {
			ioProfileID = types.StringValue(*iop.Id)
		}
	}
	wwn := types.StringNull()
	if disk.Wwn != nil {
		wwn = types.StringValue(*disk.Wwn)
	}
	diskState := types.StringNull()
	if disk.State != nil {
		diskState = types.StringValue(string(*disk.State))
	}
	boot := types.BoolValue(false)
	if bootFlag != nil {
		boot = types.BoolValue(*bootFlag)
	}

	return types.ObjectValue(vmDiskAttrTypes, map[string]attr.Value{
		"id":            id,
		"name":          name,
		"size_in_gb":    sizeInGB,
		"storage_speed": storageSpeed,
		"bus_type":      busType,
		"io_profile_id": ioProfileID,
		"wwn":           wwn,
		"state":         diskState,
		"boot":          boot,
	})
}
