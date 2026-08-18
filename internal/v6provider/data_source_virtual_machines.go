package v6provider

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	core "github.com/krystal/go-katapult/next/core"
)

const (
	virtualMachinesDataSourcePageSize   = 100
	virtualMachineHostnameAttributeName = "hostname"
)

type (
	VirtualMachinesDataSource struct {
		M *Meta
	}

	VirtualMachinesDataSourceModel struct {
		VirtualMachines []VirtualMachineSummaryDataSourceModel `tfsdk:"virtual_machines"`
	}

	VirtualMachineSummaryDataSourceModel struct {
		ID          types.String `tfsdk:"id"`
		Name        types.String `tfsdk:"name"`
		Hostname    types.String `tfsdk:"hostname"`
		FQDN        types.String `tfsdk:"fqdn"`
		IPAddresses types.Set    `tfsdk:"ip_addresses"`
		PackageName types.String `tfsdk:"package_name"`
	}
)

func (d *VirtualMachinesDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_virtual_machines"
}

func (d *VirtualMachinesDataSource) Configure(
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

func (d *VirtualMachinesDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists Virtual Machines belonging to the provider organization.",
		Attributes: map[string]schema.Attribute{
			"virtual_machines": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Virtual Machines ordered lexically by ID.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The unique identifier of the Virtual Machine.",
						},
						"name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The name of the Virtual Machine.",
						},
						virtualMachineHostnameAttributeName: schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The hostname of the Virtual Machine.",
						},
						"fqdn": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The fully qualified domain name of the Virtual Machine.",
						},
						"ip_addresses": schema.SetAttribute{
							Computed:            true,
							ElementType:         types.StringType,
							MarkdownDescription: "The IP addresses assigned to the Virtual Machine.",
						},
						"package_name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The name of the Virtual Machine package.",
						},
					},
				},
			},
		},
	}
}

func (d *VirtualMachinesDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var data VirtualMachinesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	virtualMachines, err := fetchAllOrganizationVirtualMachines(ctx, d.M)
	if err != nil {
		resp.Diagnostics.AddError("Virtual Machines Error", err.Error())
		return
	}

	data.VirtualMachines = virtualMachineSummaryDataSourceModels(virtualMachines)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func fetchAllOrganizationVirtualMachines(
	ctx context.Context,
	m *Meta,
) ([]core.GetOrganizationVirtualMachines200ResponseVirtualMachines, error) {
	virtualMachines := []core.GetOrganizationVirtualMachines200ResponseVirtualMachines{}
	for page := 1; ; page++ {
		res, err := m.Core.GetOrganizationVirtualMachinesWithResponse(ctx,
			&core.GetOrganizationVirtualMachinesParams{
				OrganizationSubDomain: &m.confOrganization,
				Page:                  &page,
				PerPage:               ptr(virtualMachinesDataSourcePageSize),
			})
		if err != nil {
			if res != nil {
				err = genericAPIError(err, res.Body)
			}
			return nil, err
		}
		if res == nil || res.JSON200 == nil {
			return nil, fmt.Errorf(
				"unexpected empty response listing Virtual Machines on page %d", page,
			)
		}

		pageVirtualMachines := res.JSON200.VirtualMachines
		for i := range pageVirtualMachines {
			if pageVirtualMachines[i].Id == nil {
				return nil, fmt.Errorf(
					"virtual machine on page %d at index %d has no ID", page, i,
				)
			}
		}
		virtualMachines = append(virtualMachines, pageVirtualMachines...)
		if !paginationHasNext(
			res.JSON200.Pagination,
			page,
			len(pageVirtualMachines),
			virtualMachinesDataSourcePageSize,
		) {
			break
		}
	}

	sort.SliceStable(virtualMachines, func(i, j int) bool {
		return *virtualMachines[i].Id < *virtualMachines[j].Id
	})

	return virtualMachines, nil
}

func virtualMachineSummaryDataSourceModel(
	virtualMachine *core.GetOrganizationVirtualMachines200ResponseVirtualMachines,
) VirtualMachineSummaryDataSourceModel {
	ipAddresses := make([]attr.Value, 0)
	if virtualMachine.IpAddresses != nil {
		ipAddresses = make([]attr.Value, 0, len(*virtualMachine.IpAddresses))
		for _, ipAddress := range *virtualMachine.IpAddresses {
			if ipAddress.Address != nil {
				ipAddresses = append(ipAddresses, types.StringValue(*ipAddress.Address))
			}
		}
	}

	model := VirtualMachineSummaryDataSourceModel{
		ID:          types.StringPointerValue(virtualMachine.Id),
		Name:        types.StringPointerValue(virtualMachine.Name),
		Hostname:    types.StringPointerValue(virtualMachine.Hostname),
		FQDN:        types.StringPointerValue(virtualMachine.Fqdn),
		IPAddresses: types.SetValueMust(types.StringType, ipAddresses),
		PackageName: types.StringNull(),
	}
	if virtualMachine.Package.IsSpecified() && !virtualMachine.Package.IsNull() {
		if pkg, err := virtualMachine.Package.Get(); err == nil {
			model.PackageName = types.StringPointerValue(pkg.Name)
		}
	}

	return model
}

func virtualMachineSummaryDataSourceModels(
	virtualMachines []core.GetOrganizationVirtualMachines200ResponseVirtualMachines,
) []VirtualMachineSummaryDataSourceModel {
	models := make([]VirtualMachineSummaryDataSourceModel, len(virtualMachines))
	for i := range virtualMachines {
		models[i] = virtualMachineSummaryDataSourceModel(&virtualMachines[i])
	}
	return models
}
