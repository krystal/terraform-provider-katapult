package v6provider

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	core "github.com/krystal/go-katapult/next/core"
)

const diskDataSourcePageSize = 200

type (
	DisksDataSource struct {
		M *Meta
	}

	DisksDataSourceModel struct {
		Disks []DiskSummaryDataSourceModel `tfsdk:"disks"`
	}

	DiskSummaryDataSourceModel struct {
		ID                 types.String `tfsdk:"id"`
		Name               types.String `tfsdk:"name"`
		SizeInGB           types.Int64  `tfsdk:"size_in_gb"`
		State              types.String `tfsdk:"state"`
		WWN                types.String `tfsdk:"wwn"`
		VirtualMachineID   types.String `tfsdk:"virtual_machine_id"`
		VirtualMachineFQDN types.String `tfsdk:"virtual_machine_fqdn"`
	}
)

func (d *DisksDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_disks"
}

func (d *DisksDataSource) Configure(
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
func (d *DisksDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists disks belonging to the provider organization.",
		Attributes: map[string]schema.Attribute{
			"disks": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Disks ordered lexically by ID.",
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
						stateAttributeName: schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Current state of the disk.",
						},
						"wwn": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "World Wide Name identifier of the disk.",
						},
						"virtual_machine_id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The ID of the assigned Virtual Machine, or null when unassigned.",
						},
						"virtual_machine_fqdn": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The FQDN of the assigned Virtual Machine, or null when unassigned.",
						},
					},
				},
			},
		},
	}
}

func (d *DisksDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var data DisksDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	disks, err := fetchAllOrganizationDisks(ctx, d.M)
	if err != nil {
		resp.Diagnostics.AddError("Disks Error", err.Error())
		return
	}

	data.Disks = diskSummaryDataSourceModels(disks)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func fetchAllOrganizationDisks(
	ctx context.Context,
	m *Meta,
) ([]core.GetOrganizationDisks200ResponseDisk, error) {
	disks := []core.GetOrganizationDisks200ResponseDisk{}
	for page := 1; ; page++ {
		res, err := m.Core.GetOrganizationDisksWithResponse(ctx,
			&core.GetOrganizationDisksParams{
				OrganizationSubDomain: &m.confOrganization,
				Page:                  &page,
				PerPage:               ptr(diskDataSourcePageSize),
			})
		if err != nil {
			if res != nil {
				err = genericAPIError(err, res.Body)
			}
			return nil, err
		}
		if res.JSON200 == nil {
			return nil, fmt.Errorf("unexpected empty response listing disks on page %d", page)
		}

		pageDisks := res.JSON200.Disk
		disks = append(disks, pageDisks...)
		if !paginationHasNext(res.JSON200.Pagination, page, len(pageDisks)) {
			break
		}
	}

	return disks, nil
}

func diskSummaryDataSourceModel(
	disk *core.GetOrganizationDisks200ResponseDisk,
) DiskSummaryDataSourceModel {
	model := DiskSummaryDataSourceModel{
		ID:                 types.StringPointerValue(disk.Id),
		Name:               types.StringPointerValue(disk.Name),
		SizeInGB:           intPointerValue(disk.SizeInGb),
		State:              stringerPointerValue(disk.State),
		WWN:                types.StringPointerValue(disk.Wwn),
		VirtualMachineID:   types.StringNull(),
		VirtualMachineFQDN: types.StringNull(),
	}

	if disk.VirtualMachineDisk.IsSpecified() && !disk.VirtualMachineDisk.IsNull() {
		if assignment, err := disk.VirtualMachineDisk.Get(); err == nil && assignment.VirtualMachine != nil {
			model.VirtualMachineID = types.StringPointerValue(assignment.VirtualMachine.Id)
			model.VirtualMachineFQDN = types.StringPointerValue(assignment.VirtualMachine.Fqdn)
		}
	}

	return model
}

func diskSummaryDataSourceModels(
	disks []core.GetOrganizationDisks200ResponseDisk,
) []DiskSummaryDataSourceModel {
	sort.SliceStable(disks, func(i, j int) bool {
		iID, iOK := nonNilString(disks[i].Id)
		jID, jOK := nonNilString(disks[j].Id)
		switch {
		case iOK && jOK:
			return iID < jID
		case iOK:
			return true
		default:
			return false
		}
	})

	models := make([]DiskSummaryDataSourceModel, len(disks))
	for i := range disks {
		models[i] = diskSummaryDataSourceModel(&disks[i])
	}
	return models
}

func paginationHasNext(
	pagination core.PaginationObject,
	page int,
	itemCount int,
) bool {
	if pagination.TotalPages.IsSpecified() && !pagination.TotalPages.IsNull() {
		if totalPages, err := pagination.TotalPages.Get(); err == nil {
			return page < totalPages
		}
	}

	return itemCount == diskDataSourcePageSize
}

func nonNilString(value *string) (string, bool) {
	if value == nil {
		return "", false
	}
	return *value, true
}
