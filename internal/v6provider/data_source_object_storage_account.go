package v6provider

import (
	"context"
	"errors"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
)

type ObjectStorageAccountDataSource struct {
	M *Meta
}

type ObjectStorageAccountDataSourceModel struct {
	ID                types.String `tfsdk:"id"`
	Region            types.String `tfsdk:"region"`
	ProvisioningState types.String `tfsdk:"provisioning_state"`
}

var objectStorageAccountDataSourceMarkdownDesc = strings.TrimSpace(`
Look up the object storage account for an organization in a given region.

Useful when another Terraform configuration manages the
` + "`katapult_object_storage_account`" + ` and you only need to reference
its ` + "`id`" + ` to attach buckets or access keys without managing the
account itself.

If you also manage the account in the same configuration, reference the
resource directly instead of going through this data source.
`)

func (d *ObjectStorageAccountDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_object_storage_account"
}

func (d *ObjectStorageAccountDataSource) Configure(
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

func (d *ObjectStorageAccountDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: objectStorageAccountDataSourceMarkdownDesc,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "Account identifier — the region " +
					"permalink.",
			},
			"region": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Region permalink, e.g. `uk-lon-1`. " +
					"Defaults to `uk-lon-1`.",
			},
			"provisioning_state": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "Current provisioning state of the " +
					"account: `provisioning`, `provisioned`, or `failed`.",
			},
		},
	}
}

func (d *ObjectStorageAccountDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var data ObjectStorageAccountDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := data.Region.ValueString()
	if region == "" {
		region = objectStorageAccountDefaultRegion
	}

	acct, err := getObjectStorageAccount(ctx, d.M, region)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			resp.Diagnostics.AddError(
				"Object Storage Account Not Found",
				"No object storage account exists for organization "+
					d.M.confOrganization+" in region "+region+".",
			)
			return
		}
		resp.Diagnostics.AddError(
			"Object Storage Account Read Error",
			err.Error(),
		)
		return
	}

	data.ID = types.StringValue(region)
	data.Region = types.StringValue(region)
	data.ProvisioningState = types.StringValue(
		string(deref(acct.ProvisioningState)),
	)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
