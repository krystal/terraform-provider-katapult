package v6provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/krystal/go-katapult/next/core"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

type (
	ObjectStorageAccessKeyResource struct {
		M *Meta
	}

	ObjectStorageAccessKeyResourceModel struct {
		ID              types.String `tfsdk:"id"`
		Name            types.String `tfsdk:"name"`
		Region          types.String `tfsdk:"region"`
		AllBucketsRead  types.Bool   `tfsdk:"all_buckets_read"`
		AllObjectsRead  types.Bool   `tfsdk:"all_objects_read"`
		AllObjectsWrite types.Bool   `tfsdk:"all_objects_write"`
		ReadBuckets     types.Set    `tfsdk:"read_buckets"`
		WriteBuckets    types.Set    `tfsdk:"write_buckets"`
		AccessKeyID     types.String `tfsdk:"access_key_id"`
		SecretAccessKey types.String `tfsdk:"secret_access_key"`
		ServerURL       types.String `tfsdk:"server_url"`
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
		//nolint:lll
		MarkdownDescription: strings.TrimSpace(`
Manages an access key for a Katapult object storage cluster.

Use ` + "`access_key_id`" + `, ` + "`secret_access_key`" + `, and ` + "`server_url`" + ` to configure an object storage client or SDK. Bucket-level permissions are managed via ` + "`read_key_ids`" + ` / ` + "`write_key_ids`" + ` on ` + "`katapult_object_storage_bucket`" + ` resources; ` + "`read_buckets`" + ` and ` + "`write_buckets`" + ` here reflect those associations.

The key is scoped to one object storage region. Reference the ` + "`region`" + ` attribute of a ` + "`katapult_object_storage_account`" + ` resource when the account is managed in the same configuration.

~> **Note:** ` + "`secret_access_key`" + ` is only available at creation time and cannot be retrieved again — it will be null after import. Changing ` + "`region`" + ` forces a new resource.
`),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Internal Katapult ID of the access key.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Human-readable name for the access key.",
			},
			objectStorageRegionAttributeName: schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Object storage region in which to create " +
					"the access key. Currently the only available region is " +
					"`uk-lon-1`. Changing this forces a new resource.",
				Validators: []validator.String{
					stringValidatorNotEmpty(),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"all_buckets_read": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Allow this key to list all buckets " +
					"in the cluster. Defaults to `false`.",
				Default: booldefault.StaticBool(false),
			},
			"all_objects_read": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Allow this key to read objects across " +
					"all buckets in the cluster. Defaults to `false`.",
				Default: booldefault.StaticBool(false),
			},
			"all_objects_write": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Allow this key to write objects across " +
					"all buckets in the cluster. Defaults to `false`.",
				Default: booldefault.StaticBool(false),
			},
			"read_buckets": schema.SetAttribute{
				Computed: true,
				MarkdownDescription: "Bucket names this key can read from. " +
					"Populated via a bucket's `read_key_ids`.",
				ElementType: types.StringType,
			},
			"write_buckets": schema.SetAttribute{
				Computed: true,
				MarkdownDescription: "Bucket names this key can write to. " +
					"Populated via a bucket's `write_key_ids`.",
				ElementType: types.StringType,
			},
			"access_key_id": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "Access key ID for " +
					"authenticating object storage clients.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"secret_access_key": schema.StringAttribute{
				Computed:  true,
				Sensitive: true,
				MarkdownDescription: "Secret access key. Available " +
					"only at creation; not retrievable " +
					"via the API. Null after import.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"server_url": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "Endpoint URL for " +
					"configuring object storage clients.",
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
		body := objectStorageNoResponseBody
		if res != nil {
			body = string(res.Body)
		}

		resp.Diagnostics.AddError(
			"Object Storage Access Key Create Error",
			fmt.Sprintf("%s: %s", err.Error(), body),
		)

		return
	}
	if res == nil || res.JSON201 == nil ||
		res.JSON201.ObjectStorageAccessKey.Id == nil ||
		*res.JSON201.ObjectStorageAccessKey.Id == "" {
		body := objectStorageNoResponseBody
		status := 0
		if res != nil {
			body = string(res.Body)
			status = res.StatusCode()
		}
		resp.Diagnostics.AddError(
			"Object Storage Access Key Create Error",
			fmt.Sprintf("unexpected response (%d): %s", status, body),
		)
		return
	}

	keyID := res.JSON201.ObjectStorageAccessKey.Id
	plan.ID = types.StringValue(*keyID)
	plan.ReadBuckets = buildStringSet(nil)
	plan.WriteBuckets = buildStringSet(nil)
	plan.AccessKeyID = types.StringNull()
	plan.SecretAccessKey = types.StringNull()
	plan.ServerURL = types.StringNull()
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	type credsResponse = core.PostObjectStorageAccessKeyGenerateCredentialsResponse
	var credsRes *credsResponse
	credErr := retry.RetryContext(ctx, 5*time.Minute,
		func() *retry.RetryError {
			var callErr error
			credsRes, callErr = r.M.Core.
				PostObjectStorageAccessKeyGenerateCredentialsWithResponse(
					ctx,
					core.PostObjectStorageAccessKeyGenerateCredentialsJSONRequestBody{
						AccessKey: core.ObjectStorageAccessKeyLookup{
							Id: keyID,
						},
					},
				)
			if credsRes == nil {
				if callErr == nil {
					callErr = errors.New(
						"unexpected empty response generating credentials",
					)
				}
				return retry.NonRetryableError(callErr)
			}

			if credsRes.JSON200 != nil {
				if validationErr := validateObjectStorageAccessKeyCredentials(
					&credsRes.JSON200.ObjectStorageAccessKey,
				); validationErr != nil {
					return retry.NonRetryableError(validationErr)
				}
				return nil
			}

			// Retryable: credentials not yet provisioned, rate limited, or
			// service temporarily unavailable.
			if credsRes.JSON422 != nil || credsRes.JSON429 != nil ||
				credsRes.JSON503 != nil {
				return retry.RetryableError(
					fmt.Errorf("%s: %s",
						credsRes.HTTPResponse.Status,
						string(credsRes.Body),
					),
				)
			}

			return retry.NonRetryableError(
				fmt.Errorf("%s: %s",
					credsRes.HTTPResponse.Status,
					string(credsRes.Body),
				),
			)
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

func validateObjectStorageAccessKeyCredentials(
	key *core.ObjectStorageAccessKey,
) error {
	missing := make([]string, 0, 3)
	if !key.S3AccessKeyId.IsSpecified() || key.S3AccessKeyId.IsNull() ||
		strings.TrimSpace(key.S3AccessKeyId.MustGet()) == "" {
		missing = append(missing, "s3_access_key_id")
	}
	if !key.S3SecretAccessKey.IsSpecified() || key.S3SecretAccessKey.IsNull() ||
		strings.TrimSpace(key.S3SecretAccessKey.MustGet()) == "" {
		missing = append(missing, "s3_secret_access_key")
	}
	if !key.ServerUrl.IsSpecified() || key.ServerUrl.IsNull() ||
		strings.TrimSpace(key.ServerUrl.MustGet()) == "" {
		missing = append(missing, "server_url")
	}
	if len(missing) > 0 {
		return fmt.Errorf(
			"unexpected credentials response: missing non-empty field(s): %s",
			strings.Join(missing, ", "),
		)
	}

	return nil
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
		if errors.Is(err, core.ErrNotFound) {
			resp.State.RemoveResource(ctx)

			return
		}

		resp.Diagnostics.AddError(
			"Object Storage Access Key Read Error",
			err.Error(),
		)

		return
	}

	if res != nil && res.JSON404 != nil {
		resp.State.RemoveResource(ctx)

		return
	}
	if res == nil || res.JSON200 == nil {
		status := 0
		body := objectStorageNoResponseBody
		if res != nil {
			status = res.StatusCode()
			body = string(res.Body)
		}
		resp.Diagnostics.AddError(
			"Object Storage Access Key Read Error",
			fmt.Sprintf(
				"unexpected response (%d): %s",
				status, body,
			),
		)
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
		errorMessage := err.Error()
		if res != nil {
			errorMessage = fmt.Sprintf("%s: %s", errorMessage, string(res.Body))
		}

		resp.Diagnostics.AddError(
			"Object Storage Access Key Update Error",
			errorMessage,
		)

		return
	}
	if res == nil || res.JSON200 == nil {
		body := objectStorageNoResponseBody
		status := 0
		if res != nil {
			body = string(res.Body)
			status = res.StatusCode()
		}
		resp.Diagnostics.AddError(
			"Object Storage Access Key Update Error",
			fmt.Sprintf("unexpected response (%d): %s", status, body),
		)
		return
	}

	r.populateModel(
		&plan, &res.JSON200.ObjectStorageAccessKey, false,
	)
	plan.SecretAccessKey = state.SecretAccessKey

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
	if err != nil && !errors.Is(err, core.ErrNotFound) {
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
		model.AccessKeyID = types.StringValue(
			key.S3AccessKeyId.MustGet(),
		)
	}

	if key.ServerUrl.IsSpecified() && !key.ServerUrl.IsNull() {
		model.ServerURL = types.StringValue(key.ServerUrl.MustGet())
	}

	if includeSecret &&
		key.S3SecretAccessKey.IsSpecified() &&
		!key.S3SecretAccessKey.IsNull() {
		model.SecretAccessKey = types.StringValue(
			key.S3SecretAccessKey.MustGet(),
		)
	}
}
