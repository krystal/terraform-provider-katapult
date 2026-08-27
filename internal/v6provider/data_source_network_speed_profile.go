package v6provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	core "github.com/krystal/go-katapult/next/core"
)

const networkSpeedProfilesPageSize = 200

var _ datasource.DataSourceWithConfigValidators = (*NetworkSpeedProfileDataSource)(nil)

type (
	NetworkSpeedProfileDataSource struct {
		M *Meta
	}

	NetworkSpeedProfileDataSourceModel struct {
		ID            types.String `tfsdk:"id"`
		Name          types.String `tfsdk:"name"`
		Permalink     types.String `tfsdk:"permalink"`
		UploadSpeed   types.Int64  `tfsdk:"upload_speed"`
		DownloadSpeed types.Int64  `tfsdk:"download_speed"`
	}
)

func (d *NetworkSpeedProfileDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_network_speed_profile"
}

func (d *NetworkSpeedProfileDataSource) Configure(
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

func networkSpeedProfileSchemaAttributes(selector bool) map[string]schema.Attribute {
	id := schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "The ID of this resource.",
	}
	permalink := schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "The permalink of the network speed profile.",
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
			MarkdownDescription: "The name of the network speed profile.",
		},
		"upload_speed": schema.Int64Attribute{
			Computed: true,
			MarkdownDescription: "Upload speed in Mbit. A value of `0` " +
				"means unrestricted.",
		},
		"download_speed": schema.Int64Attribute{
			Computed: true,
			MarkdownDescription: "Download speed in Mbit. A value of `0` " +
				"means unrestricted.",
		},
	}
}

func (d *NetworkSpeedProfileDataSource) ConfigValidators(
	_ context.Context,
) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		nonEmptySelectorConfigValidator{},
	}
}

func (d *NetworkSpeedProfileDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Retrieves a network speed profile by ID or permalink.",
		Attributes:          networkSpeedProfileSchemaAttributes(true),
	}
}

func (d *NetworkSpeedProfileDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var data NetworkSpeedProfileDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	profiles, err := fetchAllOrganizationNetworkSpeedProfiles(ctx, d.M)
	if err != nil {
		resp.Diagnostics.AddError("Network Speed Profile Error", err.Error())
		return
	}

	profile, selectorName, selectorValue := findNetworkSpeedProfile(
		profiles, data.ID, data.Permalink,
	)
	if profile == nil {
		resp.Diagnostics.AddError(
			"Network Speed Profile Not Found",
			fmt.Sprintf(
				"No network speed profile with %s %q exists in organization %q.",
				selectorName, selectorValue, d.M.confOrganization,
			),
		)
		return
	}

	data = networkSpeedProfileDataSourceModel(profile)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func findNetworkSpeedProfile(
	profiles []core.NetworkSpeedProfile,
	id types.String,
	permalink types.String,
) (*core.NetworkSpeedProfile, string, string) {
	selectorName, selectorValue := selectedStringSelector(id, permalink)

	for i := range profiles {
		candidate := profiles[i].Id
		if selectorName == "permalink" {
			candidate = profiles[i].Permalink
		}
		if candidate != nil && *candidate == selectorValue {
			return &profiles[i], selectorName, selectorValue
		}
	}

	return nil, selectorName, selectorValue
}

func fetchAllOrganizationNetworkSpeedProfiles(
	ctx context.Context,
	m *Meta,
) ([]core.NetworkSpeedProfile, error) {
	profiles := []core.NetworkSpeedProfile{}
	for page := 1; ; page++ {
		res, err := m.Core.GetOrganizationNetworkSpeedProfilesWithResponse(
			ctx, &core.GetOrganizationNetworkSpeedProfilesParams{
				OrganizationSubDomain: &m.confOrganization,
				Page:                  &page,
				PerPage:               ptr(networkSpeedProfilesPageSize),
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
				"unexpected empty response listing network speed profiles on page %d",
				page,
			)
		}

		pageProfiles := res.JSON200.NetworkSpeedProfiles
		profiles = append(profiles, pageProfiles...)
		if !paginationHasNext(
			res.JSON200.Pagination,
			page,
			len(pageProfiles),
			networkSpeedProfilesPageSize,
		) {
			break
		}
	}

	return profiles, nil
}

func networkSpeedProfileDataSourceModel(
	profile *core.NetworkSpeedProfile,
) NetworkSpeedProfileDataSourceModel {
	return NetworkSpeedProfileDataSourceModel{
		ID:            types.StringPointerValue(profile.Id),
		Name:          types.StringPointerValue(profile.Name),
		Permalink:     types.StringPointerValue(profile.Permalink),
		UploadSpeed:   nullableIntValueOrZero(profile.UploadSpeedInMbit),
		DownloadSpeed: nullableIntValueOrZero(profile.DownloadSpeedInMbit),
	}
}

func nullableIntValueOrZero(value interface {
	IsSpecified() bool
	IsNull() bool
	Get() (int, error)
},
) types.Int64 {
	result := nullableIntValue(value)
	if result.IsNull() {
		return types.Int64Value(0)
	}

	return result
}
