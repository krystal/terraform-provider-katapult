package v6provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	core "github.com/krystal/go-katapult/next/core"
)

const diskTemplatesPageSize = 100

var _ datasource.DataSourceWithConfigValidators = (*DiskTemplateDataSource)(nil)

type (
	DiskTemplateDataSource struct {
		M *Meta
	}

	DiskTemplateDataSourceModel struct {
		ID              types.String `tfsdk:"id"`
		Name            types.String `tfsdk:"name"`
		Description     types.String `tfsdk:"description"`
		Permalink       types.String `tfsdk:"permalink"`
		Universal       types.Bool   `tfsdk:"universal"`
		TemplateVersion types.Int64  `tfsdk:"template_version"`
		OSFamily        types.String `tfsdk:"os_family"`
	}
)

func (d *DiskTemplateDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_disk_template"
}

func (d *DiskTemplateDataSource) Configure(
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

func diskTemplateSchemaAttributes(selector bool) map[string]schema.Attribute {
	id := schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "The ID of this resource.",
	}
	permalink := schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "The permalink of the disk template.",
	}
	if selector {
		id.Optional = true
		permalink.Optional = true
	}

	return map[string]schema.Attribute{
		"id":        id,
		"permalink": permalink,
		"name": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "The name of the disk template.",
		},
		"description": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "The description of the disk template.",
		},
		"universal": schema.BoolAttribute{
			Computed: true,
			MarkdownDescription: "Whether the disk template is available to " +
				"all organizations.",
		},
		"template_version": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "The latest disk template version number.",
		},
		"os_family": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "The operating system family name.",
		},
	}
}

func (d *DiskTemplateDataSource) ConfigValidators(
	_ context.Context,
) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		nonEmptySelectorConfigValidator{},
	}
}

func (d *DiskTemplateDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Retrieves a disk template by ID or permalink.",
		Attributes:          diskTemplateSchemaAttributes(true),
	}
}

func (d *DiskTemplateDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var data DiskTemplateDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	params := &core.GetDiskTemplateParams{}
	selectorName, selectorValue := selectedStringSelector(data.ID, data.Permalink)
	if selectorName == "id" {
		params.DiskTemplateId = data.ID.ValueStringPointer()
	} else {
		params.DiskTemplatePermalink = data.Permalink.ValueStringPointer()
	}

	res, err := d.M.Core.GetDiskTemplateWithResponse(ctx, params)
	if err != nil {
		if res != nil {
			err = genericAPIError(err, res.Body)
		}
		resp.Diagnostics.AddError("Disk Template Error", err.Error())
		return
	}
	if res == nil || res.JSON200 == nil {
		detail := fmt.Sprintf(
			"No disk template with %s %q exists.", selectorName, selectorValue,
		)
		if res != nil {
			if apiErr := parseGenericAPIError(res.Body); apiErr != nil {
				detail = apiErr.Error()
			}
		}
		resp.Diagnostics.AddError("Disk Template Not Found", detail)
		return
	}

	data = diskTemplateDataSourceModelFromGet(&res.JSON200.DiskTemplate)
	if data.ID.IsNull() {
		resp.Diagnostics.AddError(
			"Disk Template Error",
			"disk template response did not include an ID",
		)
		return
	}

	template := &res.JSON200.DiskTemplate
	if template.LatestVersion.IsSpecified() && !template.LatestVersion.IsNull() {
		version, versionErr := template.LatestVersion.Get()
		if versionErr != nil {
			resp.Diagnostics.AddError("Disk Template Error", versionErr.Error())
			return
		}
		if version.Id != nil {
			data.TemplateVersion, versionErr = fetchDiskTemplateVersionNumber(
				ctx, d.M, *version.Id,
			)
			if versionErr != nil {
				resp.Diagnostics.AddError("Disk Template Version Error", versionErr.Error())
				return
			}
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func fetchDiskTemplateVersionNumber(
	ctx context.Context,
	meta *Meta,
	versionID string,
) (types.Int64, error) {
	res, err := meta.Core.GetDiskTemplateVersionWithResponse(
		ctx,
		&core.GetDiskTemplateVersionParams{DiskTemplateVersionId: &versionID},
	)
	if err != nil {
		if res != nil {
			err = genericAPIError(err, res.Body)
		}
		return types.Int64Null(), err
	}
	if res == nil || res.JSON200 == nil {
		detail := fmt.Sprintf(
			"unexpected empty response fetching disk template version %s",
			versionID,
		)
		if res != nil {
			if apiErr := parseGenericAPIError(res.Body); apiErr != nil {
				detail = apiErr.Error()
			}
		}
		return types.Int64Null(), fmt.Errorf("%s", detail)
	}

	return intPointerValue(res.JSON200.DiskTemplateVersion.Number), nil
}

func diskTemplateDataSourceModelFromGet(
	template *core.GetDiskTemplate200ResponseDiskTemplate,
) DiskTemplateDataSourceModel {
	model := newDiskTemplateDataSourceModel(
		template.Id,
		template.Name,
		template.Permalink,
		nullableStringValueOrEmpty(template.Description),
		types.BoolPointerValue(template.Universal),
	)

	if template.OperatingSystem.IsSpecified() && !template.OperatingSystem.IsNull() {
		if operatingSystem, err := template.OperatingSystem.Get(); err == nil {
			model.OSFamily = types.StringPointerValue(operatingSystem.Name)
		}
	}

	return model
}

func newDiskTemplateDataSourceModel(
	id *string,
	name *string,
	permalink *string,
	description types.String,
	universal types.Bool,
) DiskTemplateDataSourceModel {
	return DiskTemplateDataSourceModel{
		ID:              types.StringPointerValue(id),
		Name:            types.StringPointerValue(name),
		Description:     description,
		Permalink:       types.StringPointerValue(permalink),
		Universal:       universal,
		TemplateVersion: types.Int64Null(),
		OSFamily:        types.StringNull(),
	}
}

func nullableStringValueOrEmpty(value interface {
	IsSpecified() bool
	IsNull() bool
	Get() (string, error)
},
) types.String {
	if !value.IsSpecified() || value.IsNull() {
		return types.StringValue("")
	}
	if v, err := value.Get(); err == nil {
		return types.StringValue(v)
	}
	return types.StringValue("")
}
