package v6provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
)

type (
	TagsDataSource struct {
		M *Meta
	}

	TagsDataSourceModel struct {
		Tags types.List `tfsdk:"tags"`
	}
)

func (r TagsDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_tags"
}

func (r *TagsDataSource) Configure(
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

	r.M = meta
}

func (r TagsDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"tags": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description: "The unique identifier for the tag.",
							Required:    true,
						},
						"name": schema.StringAttribute{
							Description: "The name of the tag.",
							Computed:    true,
						},
						"color": schema.StringAttribute{
							Description: "The color of the tag.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

func (r TagsDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var data TagsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tags := []core.GetOrganizationTags200ResponseTags{}
	totalPages := 2
	for page := 1; page <= totalPages; page++ {
		res, err := r.M.Core.GetOrganizationTagsWithResponse(ctx,
			&core.GetOrganizationTagsParams{
				OrganizationSubDomain: &r.M.confOrganization,
				Page:                  &page,
				PerPage:               ptr(200),
			})
		if err != nil {
			if res != nil {
				err = genericAPIError(err, res.Body)
			}

			resp.Diagnostics.AddError("Tags Error", err.Error())
			return
		}

		totalPages, _ = res.JSON200.Pagination.TotalPages.Get()

		tags = append(tags, res.JSON200.Tags...)
	}

	attrs := make([]attr.Value, len(tags))

	attrType := map[string]attr.Type{
		"id":    types.StringType,
		"name":  types.StringType,
		"color": types.StringType,
	}

	for i, tag := range tags {
		attrs[i] = types.ObjectValueMust(
			attrType,
			map[string]attr.Value{
				"id":    types.StringPointerValue(tag.Id),
				"name":  types.StringPointerValue(tag.Name),
				"color": types.StringValue(string(*tag.Color)),
			},
		)
	}

	data.Tags = types.ListValueMust(
		types.ObjectType{
			AttrTypes: attrType,
		},
		attrs,
	)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
