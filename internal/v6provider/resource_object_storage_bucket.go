package v6provider

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/krystal/go-katapult/next/core"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

type (
	ObjectStorageBucketResource struct {
		M *Meta
	}

	ObjectStorageBucketResourceModel struct {
		Name            types.String `tfsdk:"name"`
		Region          types.String `tfsdk:"region"`
		Label           types.String `tfsdk:"label"`
		PublicURL       types.String `tfsdk:"public_url"`
		ServeStaticSite types.Bool   `tfsdk:"serve_static_site"`
		StaticSiteError types.String `tfsdk:"static_site_error"`
		StaticSiteIndex types.String `tfsdk:"static_site_index"`
		AllKeysRead     types.Bool   `tfsdk:"all_keys_read"`
		AllKeysWrite    types.Bool   `tfsdk:"all_keys_write"`
		PublicList      types.Bool   `tfsdk:"public_list"`
		PublicRead      types.Bool   `tfsdk:"public_read"`
		ReadKeyIDs      types.Set    `tfsdk:"read_key_ids"`
		WriteKeyIDs     types.Set    `tfsdk:"write_key_ids"`
	}
)

func (r *ObjectStorageBucketResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_object_storage_bucket"
}

func (r *ObjectStorageBucketResource) Configure(
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

func (r *ObjectStorageBucketResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required: true,
				Description: "The name of the bucket. " +
					"This must globally unique.",
			},
			"region": schema.StringAttribute{
				Required:    true,
				Description: "The region in which the bucket will be created.",
			},
			"label": schema.StringAttribute{
				Optional:    true,
				Description: "The label for the bucket.",
			},
			"public_url": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The public URL for the bucket.",
			},
			"serve_static_site": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Serve the bucket as a static site.",
				Default:     booldefault.StaticBool(false),
			},
			"static_site_error": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Description: "The static site error page. " +
					"Errors will be redirected to " +
					"[HTTP STATUS CODE][this value]",
				Default: stringdefault.StaticString(
					"",
				),
			},
			"static_site_index": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The static site index page.",
				Default:     stringdefault.StaticString(""),
			},
			"all_keys_read": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Allow all keys to read from the bucket.",
				Default:     booldefault.StaticBool(false),
			},
			"all_keys_write": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Allow all keys to write to the bucket.",
				Default:     booldefault.StaticBool(false),
			},
			"public_list": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Allow the bucket items to be listed publicly.",
				Default:     booldefault.StaticBool(false),
			},
			"public_read": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Allow the bucket items to be read publicly.",
				Default:     booldefault.StaticBool(false),
			},
			"read_key_ids": schema.SetAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The key IDs that can read from the bucket.",
				ElementType: types.StringType,
				Default: setdefault.StaticValue(
					types.SetValueMust(types.StringType, []attr.Value{}),
				),
			},
			"write_key_ids": schema.SetAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The key IDs that can write to the bucket.",
				ElementType: types.StringType,
				Default: setdefault.StaticValue(
					types.SetValueMust(types.StringType, []attr.Value{}),
				),
			},
		},
	}
}

func (r *ObjectStorageBucketResource) ValidateConfig(
	ctx context.Context,
	req resource.ValidateConfigRequest,
	resp *resource.ValidateConfigResponse,
) {
	var data ObjectStorageBucketResourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.ServeStaticSite.ValueBool() {
		if data.StaticSiteIndex.IsNull() || data.StaticSiteIndex.IsUnknown() {
			resp.Diagnostics.AddAttributeError(
				path.Root("static_site_index"),
				"Missing Attribute Configuration",
				//nolint:lll // minor
				"Expected static_site_index to be present when serve_static_site is true",
			)
		}

		if data.PublicList.IsNull() ||
			data.PublicList.IsUnknown() ||
			!data.PublicList.ValueBool() {
			resp.Diagnostics.AddAttributeError(
				path.Root("public_list"),
				"Missing Attribute Configuration",
				//nolint:lll // minor
				"Expected public_list to be true when serve_static_site is true",
			)
		}

		if data.PublicRead.IsNull() ||
			data.PublicRead.IsUnknown() ||
			!data.PublicRead.ValueBool() {
			resp.Diagnostics.AddAttributeError(
				path.Root("public_read"),
				"Missing Attribute Configuration",
				//nolint:lll // minor
				"Expected public_read to be true when serve_static_site is true",
			)
		}
	} else {
		if !data.StaticSiteIndex.IsNull() && !data.StaticSiteIndex.IsUnknown() {
			resp.Diagnostics.AddAttributeError(
				path.Root("static_site_index"),
				"Invalid Attribute Configuration",
				//nolint:lll // minor
				"Expected static_site_index to not be present when serve_static_site is false",
			)
		}

		if !data.StaticSiteError.IsNull() && !data.StaticSiteError.IsUnknown() {
			resp.Diagnostics.AddAttributeError(
				path.Root("static_site_error"),
				"Invalid Attribute Configuration",
				//nolint:lll // minor
				"Expected static_site_error to not be present when serve_static_site is false",
			)
		}
	}
}

func (r *ObjectStorageBucketResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan ObjectStorageBucketResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := r.M.UseOrGenerateName(plan.Name.ValueString())

	// Create the object storage account if it doesn't exist
	if err := r.createObjStoreAccountIfNotExists(
		ctx,
		plan.Region.ValueString(),
	); err != nil {
		resp.Diagnostics.AddError(
			"Object Storage Account Creation Error",
			err.Error(),
		)
		return
	}

	readKeyIDs := []string{}
	writeKeyIDs := []string{}

	resp.Diagnostics.Append(
		plan.ReadKeyIDs.ElementsAs(ctx, &readKeyIDs, false)...,
	)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(
		plan.WriteKeyIDs.ElementsAs(ctx, &writeKeyIDs, false)...,
	)
	if resp.Diagnostics.HasError() {
		return
	}

	args := core.
		PostOrganizationObjectStorageObjectStorageClusterBucketsJSONRequestBody{
		ObjectStorageCluster: core.ObjectStorageClusterLookup{
			Region: plan.Region.ValueStringPointer(),
		},
		Organization: core.OrganizationLookup{
			SubDomain: &r.M.confOrganization,
		},
		Properties: core.ObjectStorageBucketArguments{
			Name:            &name,
			Label:           plan.Label.ValueStringPointer(),
			ServeStaticSite: plan.ServeStaticSite.ValueBoolPointer(),
			StaticSiteError: plan.StaticSiteError.ValueStringPointer(),
			StaticSiteIndex: plan.StaticSiteIndex.ValueStringPointer(),
			AccessControlList: &core.ObjectStorageBucketACLArguments{
				AllKeysRead:  plan.AllKeysRead.ValueBoolPointer(),
				AllKeysWrite: plan.AllKeysWrite.ValueBoolPointer(),
				PublicList:   plan.PublicList.ValueBoolPointer(),
				PublicRead:   plan.PublicRead.ValueBoolPointer(),
				ReadKeyIds:   &readKeyIDs,
				WriteKeyIds:  &writeKeyIDs,
			},
		},
	}

	//nolint:lll // this is a generated function name.
	res, err := r.M.Core.
		PostOrganizationObjectStorageObjectStorageClusterBucketsWithResponse(ctx, args)
	if err != nil {
		resp.Diagnostics.AddError(
			"error creating object storage bucket",
			fmt.Sprintf("%s: %s", err.Error(), string(res.Body)))
		return
	}

	if err := r.ObjectStorageBucketRead(
		ctx,
		name,
		plan.Region.ValueString(),
		&plan,
	); err != nil {
		resp.Diagnostics.AddError(
			"Object Storage Bucket Read Error",
			err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *ObjectStorageBucketResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state ObjectStorageBucketResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.ObjectStorageBucketRead(
		ctx,
		state.Name.ValueString(),
		state.Region.ValueString(),
		&state,
	); err != nil {
		resp.Diagnostics.AddError(
			"Object Storage Bucket Read Error",
			err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// /nolint:lll // a lot of generated types leading to long lines.
func (r *ObjectStorageBucketResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan ObjectStorageBucketResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state ObjectStorageBucketResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	args := core.PatchObjectStorageObjectStorageClusterBucketJSONRequestBody{
		Bucket: core.ObjectStorageBucketLookup{
			Name: state.Name.ValueStringPointer(),
		},
		ObjectStorageCluster: core.ObjectStorageClusterLookup{
			Region: state.Region.ValueStringPointer(),
		},
		Properties: core.ObjectStorageBucketArguments{
			AccessControlList: &core.ObjectStorageBucketACLArguments{},
		},
	}

	if !plan.Name.Equal(state.Name) {
		args.Properties.Name = plan.Name.ValueStringPointer()
	}

	if !plan.Label.Equal(state.Label) {
		args.Properties.Label = plan.Label.ValueStringPointer()
	}

	if !plan.ServeStaticSite.Equal(state.ServeStaticSite) {
		args.Properties.ServeStaticSite = plan.ServeStaticSite.ValueBoolPointer()
	}

	if !plan.StaticSiteError.Equal(state.StaticSiteError) {
		args.Properties.StaticSiteError = plan.StaticSiteError.ValueStringPointer()
	}

	if !plan.StaticSiteIndex.Equal(state.StaticSiteIndex) {
		args.Properties.StaticSiteIndex = plan.StaticSiteIndex.ValueStringPointer()
	}

	if !plan.AllKeysRead.Equal(state.AllKeysRead) {
		args.Properties.AccessControlList.AllKeysRead = plan.AllKeysRead.ValueBoolPointer()
	}

	if !plan.AllKeysWrite.Equal(state.AllKeysWrite) {
		args.Properties.AccessControlList.AllKeysWrite = plan.AllKeysWrite.ValueBoolPointer()
	}

	if !plan.PublicList.Equal(state.PublicList) {
		args.Properties.AccessControlList.PublicList = plan.PublicList.ValueBoolPointer()
	}

	if !plan.PublicRead.Equal(state.PublicRead) {
		args.Properties.AccessControlList.PublicRead = plan.PublicRead.ValueBoolPointer()
	}

	if !plan.ReadKeyIDs.Equal(state.ReadKeyIDs) {
		readKeyIDs := []string{}
		resp.Diagnostics.Append(plan.ReadKeyIDs.ElementsAs(ctx, &readKeyIDs, false)...)
		if resp.Diagnostics.HasError() {
			return
		}

		args.Properties.AccessControlList.ReadKeyIds = &readKeyIDs
	}

	if !plan.WriteKeyIDs.Equal(state.WriteKeyIDs) {
		writeKeyIDs := []string{}
		resp.Diagnostics.Append(plan.WriteKeyIDs.ElementsAs(ctx, &writeKeyIDs, false)...)
		if resp.Diagnostics.HasError() {
			return
		}

		args.Properties.AccessControlList.WriteKeyIds = &writeKeyIDs
	}

	res, err := r.M.Core.PatchObjectStorageObjectStorageClusterBucketWithResponse(ctx, args)
	if err != nil {
		resp.Diagnostics.AddError("Object Storage Bucket Update Error", fmt.Sprintf("%s: %s", err.Error(), string(res.Body)))
		return
	}

	if err := r.ObjectStorageBucketRead(ctx, plan.Name.ValueString(), plan.Region.ValueString(), &plan); err != nil {
		resp.Diagnostics.AddError("Object Storage Bucket Read Error", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *ObjectStorageBucketResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	state := ObjectStorageBucketResourceModel{}
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.M.Core.
		DeleteObjectStorageObjectStorageClusterBucketWithResponse(ctx,
			core.DeleteObjectStorageObjectStorageClusterBucketJSONRequestBody{
				Bucket: core.ObjectStorageBucketLookup{
					Name: state.Name.ValueStringPointer(),
				},
				ObjectStorageCluster: core.ObjectStorageClusterLookup{
					Region: state.Region.ValueStringPointer(),
				},
			})
	if err != nil {
		resp.Diagnostics.AddError(
			"Object Storage Bucket Delete Error",
			err.Error(),
		)
	}
}

func (r *ObjectStorageBucketResource) ObjectStorageBucketRead(
	ctx context.Context,
	name string,
	region string,
	model *ObjectStorageBucketResourceModel,
) error {
	res, err := r.M.Core.
		GetObjectStorageObjectStorageClusterBucketWithResponse(
			ctx,
			&core.GetObjectStorageObjectStorageClusterBucketParams{
				ObjectStorageClusterRegion: &region,
				BucketName:                 &name,
			})
	if err != nil {
		return err
	}

	b := res.JSON200.ObjectStorageBucket

	model.Region = types.StringValue(region)
	model.Name = types.StringPointerValue(b.Name)

	if b.Label.IsSpecified() {
		model.Label = types.StringValue(b.Label.MustGet())
	}

	model.PublicURL = types.StringPointerValue(b.PublicUrl)
	model.ServeStaticSite = types.BoolPointerValue(b.ServeStaticSite)

	if b.StaticSiteError.IsSpecified() && !b.StaticSiteError.IsNull() {
		model.StaticSiteError = types.StringValue(b.StaticSiteError.MustGet())
	}

	if b.StaticSiteIndex.IsSpecified() && !b.StaticSiteIndex.IsNull() {
		model.StaticSiteIndex = types.StringValue(b.StaticSiteIndex.MustGet())
	}

	acl := b.AccessControlList

	model.AllKeysRead = types.BoolPointerValue(acl.AllKeysRead)
	model.AllKeysWrite = types.BoolPointerValue(acl.AllKeysWrite)
	model.PublicList = types.BoolPointerValue(acl.PublicList)
	model.PublicRead = types.BoolPointerValue(acl.PublicRead)

	if b.AccessControlList.ReadKeyIds != nil {
		model.ReadKeyIDs = buildStringSet(*acl.ReadKeyIds)
	}

	if b.AccessControlList.WriteKeyIds != nil {
		model.WriteKeyIDs = buildStringSet(*acl.WriteKeyIds)
	}

	return nil
}

func (r *ObjectStorageBucketResource) objectStorageAccountGet(
	ctx context.Context,
	region string,
) (core.ObjectStorageAccountProvisioningStateEnum, error) {
	res, err := r.M.Core.
		GetOrganizationObjectStorageObjectStorageClusterWithResponse(ctx,
			&core.GetOrganizationObjectStorageObjectStorageClusterParams{
				OrganizationSubDomain:      &r.M.confOrganization,
				ObjectStorageClusterRegion: &region,
			})
	if err != nil {
		return core.ObjectStorageAccountProvisioningStateEnumFailed,
			fmt.Errorf("%w: %s", err, string(res.Body))
	}

	return *res.JSON200.ObjectStorageAccount.ProvisioningState, nil
}

func (r *ObjectStorageBucketResource) objectStorageAccountCreate(
	ctx context.Context,
	region string,
) error {
	res, err := r.M.Core.
		PostOrganizationObjectStorageObjectStorageClusterWithResponse(ctx,
			///nolint:lll // this is a generated type.
			core.PostOrganizationObjectStorageObjectStorageClusterJSONRequestBody{
				ObjectStorageCluster: core.ObjectStorageClusterLookup{
					Region: &region,
				},
				Organization: core.OrganizationLookup{
					SubDomain: &r.M.confOrganization,
				},
			})
	if err != nil {
		return fmt.Errorf("%w: %s", err, string(res.Body))
	}

	return nil
}

func (r *ObjectStorageBucketResource) createObjStoreAccountIfNotExists(
	ctx context.Context,
	region string,
) error {
	state, err := r.objectStorageAccountGet(ctx, region)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			createErr := r.objectStorageAccountCreate(ctx, region)
			if createErr != nil {
				return createErr
			}
		} else {
			return err
		}
	}

	if state == core.ObjectStorageAccountProvisioningStateEnumProvisioned {
		return nil
	}

	waiter := &retry.StateChangeConf{
		Pending: []string{
			string(core.ObjectStorageAccountProvisioningStateEnumFailed),
			string(core.ObjectStorageAccountProvisioningStateEnumProvisioning),
		},
		Target: []string{
			string(core.ObjectStorageAccountProvisioningStateEnumProvisioned),
		},
		Refresh: func() (interface{}, string, error) {
			state, stateErr := r.objectStorageAccountGet(ctx, region)
			if err != nil {
				return 0, "", stateErr
			}

			return 1,
				string(state),
				nil
		},
		Timeout:                   5 * time.Minute,
		Delay:                     2 * time.Second,
		MinTimeout:                5 * time.Second,
		ContinuousTargetOccurence: 1,
	}

	_, err = waiter.WaitForStateContext(ctx)
	if err != nil {
		return err
	}

	return nil
}

// func (r *ObjectStorageBucketResource) objectStorageAccountDelete(
// 	_ context.Context,
// 	_ string,
// ) error {
// 	return nil
// }

/// HELPERS

func buildStringSet(in []string) basetypes.SetValue {
	values := make([]attr.Value, len(in))
	for i, v := range in {
		values[i] = types.StringValue(v)
	}

	return types.SetValueMust(types.StringType, values)
}
