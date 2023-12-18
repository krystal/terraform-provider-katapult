package v6provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type (
	LoadBalancerRuleDataSource struct {
		M *Meta
	}

	LoadBalancerRuleDataSourceModel struct {
		ID                types.String `tfsdk:"id"`
		LoadBalancerID    types.String `tfsdk:"load_balancer_id"`
		Algorithm         types.String `tfsdk:"algorithm"`
		DestinationPort   types.Int64  `tfsdk:"destination_port"`
		ListenPort        types.Int64  `tfsdk:"listen_port"`
		Protocol          types.String `tfsdk:"protocol"`
		ProxyProtocol     types.Bool   `tfsdk:"proxy_protocol"`
		Certificates      types.List   `tfsdk:"certificates"`
		BackendSSL        types.Bool   `tfsdk:"backend_ssl"`
		PassthroughSSL    types.Bool   `tfsdk:"passthrough_ssl"`
		CheckEnabled      types.Bool   `tfsdk:"check_enabled"`
		CheckFall         types.Int64  `tfsdk:"check_fall"`
		CheckInterval     types.Int64  `tfsdk:"check_interval"`
		CheckHTTPStatuses types.String `tfsdk:"check_http_statuses"`
		CheckPath         types.String `tfsdk:"check_path"`
		CheckProtocol     types.String `tfsdk:"check_protocol"`
		CheckRise         types.Int64  `tfsdk:"check_rise"`
		CheckTimeout      types.Int64  `tfsdk:"check_timeout"`
	}
)

func (ds *LoadBalancerRuleDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_load_balancer_rule"
}

func (ds *LoadBalancerRuleDataSource) Configure(
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

	ds.M = meta
}

func loadBalancerRuleDataSourceSchemaAttrs() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Required: true,
		},
		"load_balancer_id": schema.StringAttribute{
			Computed: true,
		},
		"algorithm": schema.StringAttribute{
			Computed: true,
		},
		"destination_port": schema.Int64Attribute{
			Computed: true,
		},
		"listen_port": schema.Int64Attribute{
			Computed: true,
		},
		"protocol": schema.StringAttribute{
			Computed: true,
		},
		"proxy_protocol": schema.BoolAttribute{
			Computed: true,
		},
		"certificates": schema.ListNestedAttribute{
			Computed: true,
			NestedObject: schema.NestedAttributeObject{
				Attributes: CertificateDataSourceSchemaAtrributes(),
			},
		},
		"backend_ssl": schema.BoolAttribute{
			Computed: true,
		},
		"passthrough_ssl": schema.BoolAttribute{
			Computed: true,
		},
		"check_enabled": schema.BoolAttribute{
			Computed: true,
		},
		"check_fall": schema.Int64Attribute{
			Computed: true,
		},
		"check_interval": schema.Int64Attribute{
			Computed: true,
		},
		"check_http_statuses": schema.StringAttribute{
			Computed: true,
		},
		"check_path": schema.StringAttribute{
			Computed: true,
		},
		"check_protocol": schema.StringAttribute{
			Computed: true,
		},
		"check_rise": schema.Int64Attribute{
			Computed: true,
		},
		"check_timeout": schema.Int64Attribute{
			Computed: true,
		},
	}
}

func (ds *LoadBalancerRuleDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Attributes: loadBalancerRuleDataSourceSchemaAttrs(),
	}
}

func (ds LoadBalancerRuleDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var data LoadBalancerRuleDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	id := data.ID.ValueString()

	lbr, _, err := ds.M.Core.LoadBalancerRules.GetByID(ctx,
		id)
	if err != nil {
		resp.Diagnostics.AddError(
			"Load Balancer Rule GetByID Error",
			err.Error(),
		)

		return
	}

	data.ID = types.StringValue(lbr.ID)
	data.LoadBalancerID = types.StringValue(lbr.LoadBalancer.ID)
	data.Algorithm = types.StringValue(string(lbr.Algorithm))
	data.DestinationPort = types.Int64Value(int64(lbr.DestinationPort))
	data.ListenPort = types.Int64Value(int64(lbr.ListenPort))
	data.Protocol = types.StringValue(string(lbr.Protocol))
	data.ProxyProtocol = types.BoolValue(lbr.ProxyProtocol)
	data.Certificates = types.ListValueMust(
		CertificateType(),
		ConvertCoreCertsToTFValues(lbr.Certificates),
	)
	data.BackendSSL = types.BoolValue(lbr.BackendSSL)
	data.PassthroughSSL = types.BoolValue(lbr.PassthroughSSL)
	data.CheckEnabled = types.BoolValue(lbr.CheckEnabled)
	data.CheckFall = types.Int64Value(int64(lbr.CheckFall))
	data.CheckInterval = types.Int64Value(int64(lbr.CheckInterval))
	data.CheckPath = types.StringValue(lbr.CheckPath)
	data.CheckProtocol = types.StringValue(string(lbr.CheckProtocol))
	data.CheckRise = types.Int64Value(int64(lbr.CheckRise))
	data.CheckTimeout = types.Int64Value(int64(lbr.CheckTimeout))
	data.CheckHTTPStatuses = types.StringValue(string(lbr.CheckHTTPStatuses))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
