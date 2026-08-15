package v6provider

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	core "github.com/krystal/go-katapult/next/core"
)

var _ datasource.DataSourceWithConfigValidators = (*DiskIOProfileDataSource)(nil)

type (
	DiskIOProfileDataSource struct {
		M *Meta
	}

	DiskIOProfileDataSourceModel struct {
		ID        types.String `tfsdk:"id"`
		Permalink types.String `tfsdk:"permalink"`
		Name      types.String `tfsdk:"name"`
		SpeedInMB types.Int64  `tfsdk:"speed_in_mb"`
		IOPS      types.Int64  `tfsdk:"iops"`
	}
)

func (d *DiskIOProfileDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_disk_io_profile"
}

func (d *DiskIOProfileDataSource) Configure(
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

func diskIOProfileSchemaAttributes(selector bool) map[string]schema.Attribute {
	id := schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "The unique identifier of the disk I/O profile.",
	}
	permalink := schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "The permalink of the disk I/O profile.",
	}
	if selector {
		id.Optional = true
		id.Validators = []validator.String{
			stringValidatorNotEmpty(),
		}
		permalink.Optional = true
		permalink.Validators = []validator.String{
			stringValidatorNotEmpty(),
		}
	}

	return map[string]schema.Attribute{
		"id":        id,
		"permalink": permalink,
		"name": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "The name of the disk I/O profile.",
		},
		"speed_in_mb": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "The maximum throughput in MB/s, or null when unlimited.",
		},
		"iops": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "The maximum I/O operations per second, or null when unlimited.",
		},
	}
}

func (d *DiskIOProfileDataSource) ConfigValidators(
	_ context.Context,
) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("permalink"),
		),
	}
}

func (d *DiskIOProfileDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Retrieves a disk I/O profile by ID or permalink.",
		Attributes:          diskIOProfileSchemaAttributes(true),
	}
}

func (d *DiskIOProfileDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var data DiskIOProfileDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	profiles, err := fetchAllOrganizationDiskIOProfiles(ctx, d.M)
	if err != nil {
		resp.Diagnostics.AddError("Disk I/O Profile Error", err.Error())
		return
	}

	profile, selectorName, selectorValue := findDiskIOProfile(
		profiles, data.ID, data.Permalink,
	)
	if profile != nil {
		data = diskIOProfileDataSourceModel(profile)
		resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
		return
	}

	resp.Diagnostics.AddError(
		"Disk I/O Profile Not Found",
		fmt.Sprintf("No disk I/O profile with %s %q exists in organization %q.",
			selectorName, selectorValue, d.M.confOrganization),
	)
}

func findDiskIOProfile(
	profiles []core.DiskIOProfile,
	id types.String,
	permalink types.String,
) (*core.DiskIOProfile, string, string) {
	selectorName := "id"
	selectorValue := id.ValueString()
	if id.IsNull() {
		selectorName = "permalink"
		selectorValue = permalink.ValueString()
	}

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

func fetchAllOrganizationDiskIOProfiles(
	ctx context.Context,
	m *Meta,
) ([]core.DiskIOProfile, error) {
	profiles := []core.DiskIOProfile{}
	for page := 1; ; page++ {
		res, err := m.Core.GetOrganizationDiskIoProfilesWithResponse(ctx,
			&core.GetOrganizationDiskIoProfilesParams{
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
			return nil, fmt.Errorf("unexpected empty response listing disk I/O profiles on page %d", page)
		}

		pageProfiles := res.JSON200.DiskIoProfiles
		profiles = append(profiles, pageProfiles...)
		if !paginationHasNext(res.JSON200.Pagination, page, len(pageProfiles)) {
			break
		}
	}

	sort.SliceStable(profiles, func(i, j int) bool {
		iID, iOK := nonNilString(profiles[i].Id)
		jID, jOK := nonNilString(profiles[j].Id)
		switch {
		case iOK && jOK:
			return iID < jID
		case iOK:
			return true
		default:
			return false
		}
	})
	return profiles, nil
}

func diskIOProfileDataSourceModel(
	profile *core.DiskIOProfile,
) DiskIOProfileDataSourceModel {
	return DiskIOProfileDataSourceModel{
		ID:        types.StringPointerValue(profile.Id),
		Permalink: types.StringPointerValue(profile.Permalink),
		Name:      types.StringPointerValue(profile.Name),
		SpeedInMB: nullableIntValue(profile.SpeedInMb),
		IOPS:      nullableIntValue(profile.Iops),
	}
}

func nullableIntValue(value interface {
	IsSpecified() bool
	IsNull() bool
	Get() (int, error)
},
) types.Int64 {
	if !value.IsSpecified() || value.IsNull() {
		return types.Int64Null()
	}
	if v, err := value.Get(); err == nil {
		return types.Int64Value(int64(v))
	}
	return types.Int64Null()
}
