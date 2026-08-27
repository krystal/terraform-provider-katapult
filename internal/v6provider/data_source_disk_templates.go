package v6provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	core "github.com/krystal/go-katapult/next/core"
)

type (
	DiskTemplatesDataSource struct {
		M *Meta
	}

	DiskTemplatesDataSourceModel struct {
		ID               types.String                  `tfsdk:"id"`
		IncludeUniversal types.Bool                    `tfsdk:"include_universal"`
		Templates        []DiskTemplateDataSourceModel `tfsdk:"templates"`
	}
)

func (d *DiskTemplatesDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_disk_templates"
}

func (d *DiskTemplatesDataSource) Configure(
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

func (d *DiskTemplatesDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists disk templates available to the provider organization.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Always set to the provider organization value.",
			},
			"include_universal": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Include universal disk templates. " +
					"Defaults to `true`.",
			},
			"templates": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The available disk templates.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: diskTemplateSchemaAttributes(false),
				},
			},
		},
	}
}

func (d *DiskTemplatesDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var data DiskTemplatesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	includeUniversal := true
	if !data.IncludeUniversal.IsNull() && !data.IncludeUniversal.IsUnknown() {
		includeUniversal = data.IncludeUniversal.ValueBool()
	}
	templates, err := fetchAllOrganizationDiskTemplates(
		ctx, d.M, includeUniversal,
	)
	if err != nil {
		resp.Diagnostics.AddError("Disk Templates Error", err.Error())
		return
	}

	data.ID = types.StringValue(d.M.confOrganization)
	data.IncludeUniversal = types.BoolValue(includeUniversal)
	data.Templates = make([]DiskTemplateDataSourceModel, len(templates))
	for i := range templates {
		data.Templates[i] = diskTemplateDataSourceModelFromList(&templates[i])
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func fetchAllOrganizationDiskTemplates(
	ctx context.Context,
	m *Meta,
	includeUniversal bool,
) ([]core.GetOrganizationDiskTemplates200ResponseDiskTemplates, error) {
	templates := []core.GetOrganizationDiskTemplates200ResponseDiskTemplates{}
	for page := 1; ; page++ {
		res, err := m.Core.GetOrganizationDiskTemplatesWithResponse(
			ctx, &core.GetOrganizationDiskTemplatesParams{
				OrganizationSubDomain: &m.confOrganization,
				IncludeUniversal:      &includeUniversal,
				Page:                  &page,
				PerPage:               ptr(diskTemplatesPageSize),
			},
		)
		if err != nil {
			if res != nil {
				err = genericAPIError(err, res.Body)
			}
			return nil, err
		}
		if res == nil || res.JSON200 == nil {
			if res != nil {
				if apiErr := parseGenericAPIError(res.Body); apiErr != nil {
					return nil, apiErr
				}
			}
			return nil, fmt.Errorf(
				"unexpected empty response listing disk templates on page %d",
				page,
			)
		}

		pageTemplates := res.JSON200.DiskTemplates
		templates = append(templates, pageTemplates...)
		if !paginationHasNext(
			res.JSON200.Pagination,
			page,
			len(pageTemplates),
			diskTemplatesPageSize,
		) {
			break
		}
	}

	return templates, nil
}

func diskTemplateDataSourceModelFromList(
	template *core.GetOrganizationDiskTemplates200ResponseDiskTemplates,
) DiskTemplateDataSourceModel {
	model := newDiskTemplateDataSourceModel(
		template.Id,
		template.Name,
		template.Permalink,
		nullableStringValueOrEmpty(template.Description),
		types.BoolPointerValue(template.Universal),
	)

	if template.LatestVersion.IsSpecified() && !template.LatestVersion.IsNull() {
		if version, err := template.LatestVersion.Get(); err == nil {
			model.TemplateVersion = intPointerValue(version.Number)
		}
	}
	if template.OperatingSystem.IsSpecified() && !template.OperatingSystem.IsNull() {
		if operatingSystem, err := template.OperatingSystem.Get(); err == nil {
			model.OSFamily = types.StringPointerValue(operatingSystem.Name)
		}
	}

	return model
}
