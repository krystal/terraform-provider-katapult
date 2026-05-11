package v6provider

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
)

type ObjectStorageBucketDataSource struct {
	M *Meta
}

func (d *ObjectStorageBucketDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_object_storage_bucket"
}

func (d *ObjectStorageBucketDataSource) Configure(
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

func (d *ObjectStorageBucketDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Fetch details for an existing object storage bucket.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Globally unique bucket name.",
			},
			"region": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Region permalink, e.g. " +
					"`uk-lon-1`. Defaults to `uk-lon-1`.",
			},
			"label": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Optional bucket label in Katapult.",
			},
			"public_url": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Public base URL for accessing objects in this bucket.",
			},
			"serve_static_site": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the bucket is served as a static site.",
			},
			"static_site_index": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Default index document for static site serving.",
			},
			"static_site_error": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Error document suffix for static site serving.",
			},
			"all_keys_read": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether all access keys have read permission.",
			},
			"all_keys_write": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether all access keys have write permission.",
			},
			"public_list": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether unauthenticated object listing is allowed.",
			},
			"public_read": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether unauthenticated object reads are allowed.",
			},
			"read_key_ids": schema.SetAttribute{
				Computed:            true,
				MarkdownDescription: "Access key IDs with read permission on this bucket.",
				ElementType:         types.StringType,
			},
			"write_key_ids": schema.SetAttribute{
				Computed:            true,
				MarkdownDescription: "Access key IDs with write permission on this bucket.",
				ElementType:         types.StringType,
			},
		},
	}
}

func (d *ObjectStorageBucketDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var data ObjectStorageBucketResourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.Region.IsNull() || data.Region.ValueString() == "" {
		data.Region = types.StringValue("uk-lon-1")
	}

	r := &ObjectStorageBucketResource{M: d.M}
	if err := r.ObjectStorageBucketRead(
		ctx,
		data.Name.ValueString(),
		data.Region.ValueString(),
		&data,
	); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			resp.Diagnostics.AddError(
				"Object Storage Bucket Not Found",
				err.Error(),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Object Storage Bucket Read Error",
			err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
