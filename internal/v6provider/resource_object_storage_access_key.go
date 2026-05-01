package v6provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/krystal/go-katapult/next/core"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

type (
	ObjectStorageAccessKeyResource struct {
		M *Meta
	}

	ObjectStorageAccessKeyResourceModel struct {
		ID                types.String `tfsdk:"id"`
		Name              types.String `tfsdk:"name"`
		Region            types.String `tfsdk:"region"`
		AllBucketsRead    types.Bool   `tfsdk:"all_buckets_read"`
		AllObjectsRead    types.Bool   `tfsdk:"all_objects_read"`
		AllObjectsWrite   types.Bool   `tfsdk:"all_objects_write"`
		ReadBuckets       types.Set    `tfsdk:"read_buckets"`
		WriteBuckets      types.Set    `tfsdk:"write_buckets"`
		S3AccessKeyID     types.String `tfsdk:"s3_access_key_id"`
		S3SecretAccessKey types.String `tfsdk:"s3_secret_access_key"`
		ServerURL         types.String `tfsdk:"server_url"`
	}
)

func (r *ObjectStorageAccessKeyResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_object_storage_access_key"
}

func (r *ObjectStorageAccessKeyResource) Configure(
	_ context.Context,
	req resource.ConfigureRequest,
	resp *resource.ConfigureResponse,
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

func (r *ObjectStorageAccessKeyResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The unique identifier of the access key.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the access key.",
			},
			"region": schema.StringAttribute{
				Required: true,
				Description: "The region of the object storage " +
					"cluster.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"all_buckets_read": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Description: "Allow this key to list all " +
					"buckets.",
				Default: booldefault.StaticBool(false),
			},
			"all_objects_read": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Description: "Allow this key to read objects " +
					"across all buckets.",
				Default: booldefault.StaticBool(false),
			},
			"all_objects_write": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Description: "Allow this key to write objects " +
					"across all buckets.",
				Default: booldefault.StaticBool(false),
			},
			"read_buckets": schema.SetAttribute{
				Computed: true,
				Description: "Buckets this key has read " +
					"access to. Managed via the bucket " +
					"resource's read_key_ids.",
				ElementType: types.StringType,
			},
			"write_buckets": schema.SetAttribute{
				Computed: true,
				Description: "Buckets this key has write " +
					"access to. Managed via the bucket " +
					"resource's write_key_ids.",
				ElementType: types.StringType,
			},
			"s3_access_key_id": schema.StringAttribute{
				Computed:    true,
				Description: "The S3 access key ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"s3_secret_access_key": schema.StringAttribute{
				Computed:  true,
				Sensitive: true,
				Description: "The S3 secret access key. Only " +
					"available at creation time; empty after " +
					"import.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"server_url": schema.StringAttribute{
				Computed:    true,
				Description: "The S3 server URL.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *ObjectStorageAccessKeyResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan ObjectStorageAccessKeyResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := ensureObjectStorageAccount(
		ctx, r.M, plan.Region.ValueString(),
	); err != nil {
		resp.Diagnostics.AddError(
			"Object Storage Account Creation Error",
			err.Error(),
		)

		return
	}

	//nolint:lll // generated function and type names.
	res, err := r.M.Core.PostOrganizationObjectStorageObjectStorageClusterAccessKeysWithResponse(
		ctx,
		core.PostOrganizationObjectStorageObjectStorageClusterAccessKeysJSONRequestBody{
			ObjectStorageCluster: core.ObjectStorageClusterLookup{
				Region: plan.Region.ValueStringPointer(),
			},
			Organization: core.OrganizationLookup{
				SubDomain: &r.M.confOrganization,
			},
			Properties: core.ObjectStorageAccessKeyArguments{
				Name:            plan.Name.ValueString(),
				AllBucketsRead:  plan.AllBucketsRead.ValueBoolPointer(),
				AllObjectsRead:  plan.AllObjectsRead.ValueBoolPointer(),
				AllObjectsWrite: plan.AllObjectsWrite.ValueBoolPointer(),
			},
		},
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Object Storage Access Key Create Error",
			fmt.Sprintf("%s: %s", err.Error(), string(res.Body)),
		)

		return
	}

	keyID := res.JSON201.ObjectStorageAccessKey.Id

	type credsResponse = core.PostObjectStorageAccessKeyGenerateCredentialsResponse //nolint:lll
	var credsRes *credsResponse
	credErr := retry.RetryContext(ctx, 5*time.Minute,
		func() *retry.RetryError {
			var callErr error
			credsRes, callErr = r.M.Core.
				PostObjectStorageAccessKeyGenerateCredentialsWithResponse(
					ctx,
					core.PostObjectStorageAccessKeyGenerateCredentialsJSONRequestBody{ //nolint:lll
						AccessKey: core.ObjectStorageAccessKeyLookup{
							Id: keyID,
						},
					},
				)
			if credsRes != nil {
				if credsRes.JSON200 != nil {
					return nil
				}
				if credsRes.JSON404 != nil || credsRes.JSON403 != nil {
					return retry.NonRetryableError(callErr)
				}

				return retry.RetryableError(
					fmt.Errorf("provisioning in progress"),
				)
			}

			if callErr != nil {
				return retry.NonRetryableError(callErr)
			}

			return nil
		},
	)
	if credErr != nil {
		resp.Diagnostics.AddError(
			"Object Storage Access Key Credentials Error",
			credErr.Error(),
		)

		return
	}

	r.populateModel(
		&plan, &credsRes.JSON200.ObjectStorageAccessKey, true,
	)

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *ObjectStorageAccessKeyResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state ObjectStorageAccessKeyResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	res, err := r.M.Core.GetObjectStorageAccessKeyWithResponse(
		ctx,
		&core.GetObjectStorageAccessKeyParams{
			AccessKeyId: state.ID.ValueStringPointer(),
		},
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Object Storage Access Key Read Error",
			err.Error(),
		)

		return
	}

	if res.JSON404 != nil {
		resp.State.RemoveResource(ctx)

		return
	}

	r.populateModel(
		&state, &res.JSON200.ObjectStorageAccessKey, false,
	)

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *ObjectStorageAccessKeyResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan, state ObjectStorageAccessKeyResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	args := core.PatchObjectStorageAccessKeyJSONRequestBody{
		AccessKey: core.ObjectStorageAccessKeyLookup{
			Id: state.ID.ValueStringPointer(),
		},
		Properties: core.ObjectStorageAccessKeyArguments{
			Name:            plan.Name.ValueString(),
			AllBucketsRead:  plan.AllBucketsRead.ValueBoolPointer(),
			AllObjectsRead:  plan.AllObjectsRead.ValueBoolPointer(),
			AllObjectsWrite: plan.AllObjectsWrite.ValueBoolPointer(),
		},
	}

	res, err := r.M.Core.PatchObjectStorageAccessKeyWithResponse(
		ctx, args,
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Object Storage Access Key Update Error",
			fmt.Sprintf("%s: %s", err.Error(), string(res.Body)),
		)

		return
	}

	r.populateModel(
		&plan, &res.JSON200.ObjectStorageAccessKey, false,
	)
	plan.S3SecretAccessKey = state.S3SecretAccessKey

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *ObjectStorageAccessKeyResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state ObjectStorageAccessKeyResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.M.Core.DeleteObjectStorageAccessKeyWithResponse(
		ctx,
		core.DeleteObjectStorageAccessKeyJSONRequestBody{
			AccessKey: core.ObjectStorageAccessKeyLookup{
				Id: state.ID.ValueStringPointer(),
			},
		},
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Object Storage Access Key Delete Error",
			err.Error(),
		)
	}
}

func (r *ObjectStorageAccessKeyResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *ObjectStorageAccessKeyResource) populateModel(
	model *ObjectStorageAccessKeyResourceModel,
	key *core.ObjectStorageAccessKey,
	includeSecret bool,
) {
	model.ID = types.StringPointerValue(key.Id)
	model.Name = types.StringPointerValue(key.Name)

	if key.Region != nil {
		model.Region = types.StringPointerValue(key.Region)
	}

	model.AllBucketsRead = types.BoolPointerValue(key.AllBucketsRead)
	model.AllObjectsRead = types.BoolPointerValue(key.AllObjectsRead)
	model.AllObjectsWrite = types.BoolPointerValue(key.AllObjectsWrite)

	if key.ReadBuckets != nil {
		model.ReadBuckets = buildStringSet(*key.ReadBuckets)
	} else {
		model.ReadBuckets = types.SetValueMust(
			types.StringType, []attr.Value{},
		)
	}

	if key.WriteBuckets != nil {
		model.WriteBuckets = buildStringSet(*key.WriteBuckets)
	} else {
		model.WriteBuckets = types.SetValueMust(
			types.StringType, []attr.Value{},
		)
	}

	if key.S3AccessKeyId.IsSpecified() && !key.S3AccessKeyId.IsNull() {
		model.S3AccessKeyID = types.StringValue(
			key.S3AccessKeyId.MustGet(),
		)
	}

	if key.ServerUrl.IsSpecified() && !key.ServerUrl.IsNull() {
		model.ServerURL = types.StringValue(key.ServerUrl.MustGet())
	}

	if includeSecret &&
		key.S3SecretAccessKey.IsSpecified() &&
		!key.S3SecretAccessKey.IsNull() {
		model.S3SecretAccessKey = types.StringValue(
			key.S3SecretAccessKey.MustGet(),
		)
	}
}
