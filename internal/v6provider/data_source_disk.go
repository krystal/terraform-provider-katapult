package v6provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	core "github.com/krystal/go-katapult/next/core"
)

type (
	DiskDataSource struct {
		M *Meta
	}

	DiskDataSourceModel struct {
		ID                  types.String `tfsdk:"id"`
		Name                types.String `tfsdk:"name"`
		SizeInGB            types.Int64  `tfsdk:"size_in_gb"`
		StorageSpeed        types.String `tfsdk:"storage_speed"`
		BusType             types.String `tfsdk:"bus_type"`
		IOProfileID         types.String `tfsdk:"io_profile_id"`
		WWN                 types.String `tfsdk:"wwn"`
		State               types.String `tfsdk:"state"`
		DataCenterID        types.String `tfsdk:"data_center_id"`
		DataCenterName      types.String `tfsdk:"data_center_name"`
		DataCenterPermalink types.String `tfsdk:"data_center_permalink"`
		VirtualMachineID    types.String `tfsdk:"virtual_machine_id"`
		VirtualMachineFQDN  types.String `tfsdk:"virtual_machine_fqdn"`
		Boot                types.Bool   `tfsdk:"boot"`
		AttachOnBoot        types.Bool   `tfsdk:"attach_on_boot"`
		AttachmentState     types.String `tfsdk:"attachment_state"`
	}
)

func (d *DiskDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_disk"
}

func (d *DiskDataSource) Configure(
	_ context.Context,
	req datasource.ConfigureRequest,
	resp *datasource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		return
	}

	meta, ok := req.ProviderData.(*Meta)
	if !ok {
		resp.Diagnostics.AddError("Meta Error", "meta is not of type *Meta")
		return
	}

	d.M = meta
}

//nolint:goconst // Terraform attribute names are clearer inline in schema declarations.
func (d *DiskDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Retrieves a disk and its current Virtual Machine assignment observations.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The unique identifier of the disk.",
				Validators: []validator.String{
					stringValidatorNotEmpty(),
				},
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
				MarkdownDescription: "The ID of the I/O profile applied to the disk.",
			},
			"wwn": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "World Wide Name identifier of the disk.",
			},
			stateAttributeName: schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Current state of the disk.",
			},
			"data_center_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The ID of the data center containing the disk.",
			},
			"data_center_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The name of the data center containing the disk.",
			},
			"data_center_permalink": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The permalink of the data center containing the disk.",
			},
			"virtual_machine_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The ID of the assigned Virtual Machine, or null when unassigned.",
			},
			"virtual_machine_fqdn": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The FQDN of the assigned Virtual Machine, or null when unassigned.",
			},
			"boot": schema.BoolAttribute{
				Computed: true,
				MarkdownDescription: "Whether this is the assigned Virtual " +
					"Machine's boot disk, or null when unassigned.",
			},
			"attach_on_boot": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Observed attach-on-boot policy, or null when unassigned.",
			},
			"attachment_state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Observed physical attachment state, or null when unassigned.",
			},
		},
	}
}

func (d *DiskDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var data DiskDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	requestedID := data.ID.ValueString()

	res, err := d.M.Core.GetDiskWithResponse(ctx, &core.GetDiskParams{
		DiskId: &requestedID,
	})
	if err != nil {
		if res != nil {
			err = genericAPIError(err, res.Body)
		}
		resp.Diagnostics.AddError("Disk Error", err.Error())
		return
	}
	if res.JSON200 == nil {
		resp.Diagnostics.AddError("Disk Error", "unexpected empty response fetching disk")
		return
	}

	data = diskDataSourceModel(&res.JSON200.Disk)
	if data.ID.IsNull() {
		resp.Diagnostics.AddError("Disk Error", fmt.Sprintf(
			"disk %q response did not include an ID", requestedID,
		))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func diskDataSourceModel(disk *core.GetDisk200ResponseDisk) DiskDataSourceModel {
	model := DiskDataSourceModel{
		ID:                  types.StringPointerValue(disk.Id),
		Name:                types.StringPointerValue(disk.Name),
		SizeInGB:            intPointerValue(disk.SizeInGb),
		StorageSpeed:        stringerPointerValue(disk.StorageSpeed),
		BusType:             types.StringNull(),
		IOProfileID:         types.StringNull(),
		WWN:                 types.StringPointerValue(disk.Wwn),
		State:               stringerPointerValue(disk.State),
		DataCenterID:        types.StringNull(),
		DataCenterName:      types.StringNull(),
		DataCenterPermalink: types.StringNull(),
		VirtualMachineID:    types.StringNull(),
		VirtualMachineFQDN:  types.StringNull(),
		Boot:                types.BoolNull(),
		AttachOnBoot:        types.BoolNull(),
		AttachmentState:     types.StringNull(),
	}

	if disk.BusType.IsSpecified() && !disk.BusType.IsNull() {
		if value, err := disk.BusType.Get(); err == nil {
			model.BusType = types.StringValue(string(value))
		}
	}
	if disk.IoProfile.IsSpecified() && !disk.IoProfile.IsNull() {
		if value, err := disk.IoProfile.Get(); err == nil {
			model.IOProfileID = types.StringPointerValue(value.Id)
		}
	}
	if disk.DataCenter != nil {
		model.DataCenterID = types.StringPointerValue(disk.DataCenter.Id)
		model.DataCenterName = types.StringPointerValue(disk.DataCenter.Name)
		model.DataCenterPermalink = types.StringPointerValue(disk.DataCenter.Permalink)
	}
	if disk.VirtualMachineDisk.IsSpecified() && !disk.VirtualMachineDisk.IsNull() {
		if assignment, err := disk.VirtualMachineDisk.Get(); err == nil {
			model.Boot = types.BoolPointerValue(assignment.Boot)
			model.AttachOnBoot = types.BoolPointerValue(assignment.AttachOnBoot)
			model.AttachmentState = stringerPointerValue(assignment.State)
			if assignment.VirtualMachine != nil {
				model.VirtualMachineID = types.StringPointerValue(assignment.VirtualMachine.Id)
				model.VirtualMachineFQDN = types.StringPointerValue(assignment.VirtualMachine.Fqdn)
			}
		}
	}

	return model
}

func intPointerValue(value *int) types.Int64 {
	if value == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*value))
}

func stringerPointerValue[T ~string](value *T) types.String {
	if value == nil {
		return types.StringNull()
	}
	return types.StringValue(string(*value))
}
