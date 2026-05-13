package v6provider

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/krystal/go-katapult/next/core"
)

const objectStorageAccountDefaultRegion = "uk-lon-1"

type (
	ObjectStorageAccountResource struct {
		M *Meta
	}

	ObjectStorageAccountResourceModel struct {
		ID                types.String `tfsdk:"id"`
		Region            types.String `tfsdk:"region"`
		AdoptExisting     types.Bool   `tfsdk:"adopt_existing"`
		ProvisioningState types.String `tfsdk:"provisioning_state"`
	}
)

//nolint:lll
var objectStorageAccountMarkdownDesc = strings.TrimSpace(`
Manages the object storage account for an organization in a given region.

A Katapult organization has at most one object storage account per region. This resource creates the account (if it does not already exist) and waits for it to reach the ` + "`provisioned`" + ` state. All ` + "`katapult_object_storage_bucket`" + ` and ` + "`katapult_object_storage_access_key`" + ` resources must reference this resource via ` + "`object_storage_account_id`" + `, which ties their lifecycle to the account and gives Terraform a way to clean the account up when no longer needed.

~> **Only declare one of these per (organization, region).** If your organization already has object storage enabled via the Katapult dashboard, declare this resource anyway and import the existing account — otherwise Terraform cannot clean it up on destroy, and your organization will continue to be billed.
`)

func (r *ObjectStorageAccountResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_object_storage_account"
}

func (r *ObjectStorageAccountResource) Configure(
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

func (r *ObjectStorageAccountResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: objectStorageAccountMarkdownDesc,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "Account identifier — the region " +
					"permalink. Reference this from " +
					"`katapult_object_storage_bucket` and " +
					"`katapult_object_storage_access_key` via " +
					"`object_storage_account_id`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"region": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Region permalink, e.g. `uk-lon-1`. " +
					"Defaults to `uk-lon-1`. Changing this forces " +
					"replacement.",
				Default: stringdefault.StaticString(
					objectStorageAccountDefaultRegion,
				),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"adopt_existing": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Adopt an existing object storage " +
					"account for this region if one already exists, " +
					"instead of erroring with import instructions. " +
					"Defaults to `false`. " +
					"Changing this forces replacement.",
				Default: booldefault.StaticBool(false),
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"provisioning_state": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "Current provisioning state of the " +
					"account: `provisioning`, `provisioned`, or `failed`.",
			},
		},
	}
}

func (r *ObjectStorageAccountResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan ObjectStorageAccountResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := plan.Region.ValueString()
	adopt := plan.AdoptExisting.ValueBool()

	existing, getErr := getObjectStorageAccount(ctx, r.M, region)
	switch {
	case getErr == nil:
		if !adopt {
			resp.Diagnostics.AddError(
				"Object Storage Account Already Exists",
				fmt.Sprintf(
					"An object storage account already exists for "+
						"organization %q in region %q "+
						"(provisioning_state: %s).\n\n"+
						"To adopt it into Terraform management, either:\n"+
						"  * Import it:\n"+
						"      terraform import %s %s\n"+
						"  * Or set `adopt_existing = true` on this "+
						"resource and re-run apply.\n\n"+
						"Use `adopt_existing` with care — it silently "+
						"takes ownership of any existing account in this "+
						"region, including its buckets and access keys. "+
						"Prefer import unless you are migrating an "+
						"existing setup into Terraform.",
					r.M.confOrganization, region,
					deref(existing.ProvisioningState),
					"katapult_object_storage_account.<name>",
					region,
				),
			)
			return
		}
		// Adopting — fall through to waiter, no Create call needed.
	case errors.Is(getErr, core.ErrNotFound):
		if err := createObjectStorageAccount(ctx, r.M, region); err != nil {
			resp.Diagnostics.AddError(
				"Object Storage Account Create Error",
				err.Error(),
			)
			return
		}
	default:
		resp.Diagnostics.AddError(
			"Object Storage Account Read Error",
			getErr.Error(),
		)
		return
	}

	acct, err := waitForObjectStorageAccountProvisioned(ctx, r.M, region)
	if err != nil {
		resp.Diagnostics.AddError(
			"Object Storage Account Provisioning Error",
			err.Error(),
		)
		return
	}

	plan.ID = types.StringValue(region)
	plan.Region = types.StringValue(region)
	plan.AdoptExisting = types.BoolValue(adopt)
	plan.ProvisioningState = types.StringValue(
		string(deref(acct.ProvisioningState)),
	)

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *ObjectStorageAccountResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state ObjectStorageAccountResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := state.Region.ValueString()
	if region == "" {
		region = state.ID.ValueString()
	}

	acct, err := getObjectStorageAccount(ctx, r.M, region)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Object Storage Account Read Error",
			err.Error(),
		)
		return
	}

	state.ID = types.StringValue(region)
	state.Region = types.StringValue(region)
	state.ProvisioningState = types.StringValue(
		string(deref(acct.ProvisioningState)),
	)
	// adopt_existing is a Create-time-only knob; it doesn't reflect any
	// server-side property and is left untouched by Read.

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *ObjectStorageAccountResource) Update(
	_ context.Context,
	_ resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	// All user-configurable attributes (region) force replacement, so Update
	// is never called with a real diff. This is here only to satisfy the
	// resource.Resource interface.
	resp.Diagnostics.AddError(
		"Object Storage Account Update Not Supported",
		"All configurable attributes on katapult_object_storage_account "+
			"force replacement; Update should never be called.",
	)
}

func (r *ObjectStorageAccountResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state ObjectStorageAccountResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := state.Region.ValueString()

	// Preflight: refuse to delete the account if any buckets or access keys
	// still exist in this region — managed by Terraform or not.
	if err := preflightObjectStorageAccountDelete(
		ctx, r.M, region,
	); err != nil {
		resp.Diagnostics.AddError(
			"Object Storage Account Delete Blocked",
			err.Error(),
		)
		return
	}

	delRes, err := r.M.Core.
		DeleteOrganizationObjectStorageObjectStorageClusterWithResponse(
			ctx,
			core.DeleteOrganizationObjectStorageObjectStorageClusterJSONRequestBody{
				ObjectStorageCluster: core.ObjectStorageClusterLookup{
					Region: &region,
				},
				Organization: core.OrganizationLookup{
					SubDomain: &r.M.confOrganization,
				},
			},
		)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return
		}

		body := ""
		if delRes != nil {
			body = string(delRes.Body)
		}
		resp.Diagnostics.AddError(
			"Object Storage Account Delete Error",
			fmt.Sprintf("%s: %s", err.Error(), body),
		)
		return
	}

	if r.M.SkipTrashObjectPurge {
		return
	}

	if delRes.JSON200 == nil || delRes.JSON200.TrashObject.Id == nil {
		// Nothing to purge — either the API didn't move the account to
		// trash, or the response shape is unexpected. Don't fail the
		// destroy on this.
		return
	}

	trashID := *delRes.JSON200.TrashObject.Id
	if err := purgeTrashObject(
		ctx, r.M, 5*time.Minute, core.TrashObject{Id: &trashID},
	); err != nil {
		resp.Diagnostics.AddError(
			"Failed to purge object storage account from trash.",
			err.Error(),
		)
	}
}

func (r *ObjectStorageAccountResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	region := strings.TrimSpace(req.ID)
	if region == "" {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			"Expected import ID to be a region permalink, e.g. uk-lon-1.",
		)
		return
	}

	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("id"), region)...,
	)
	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("region"), region)...,
	)
	resp.Diagnostics.Append(
		resp.State.SetAttribute(
			ctx, path.Root("adopt_existing"), false,
		)...,
	)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// getObjectStorageAccount fetches the object storage account for the given
// region. Returns core.ErrNotFound if no account exists.
func getObjectStorageAccount(
	ctx context.Context,
	m *Meta,
	region string,
) (*core.ObjectStorageAccount, error) {
	res, err := m.Core.
		GetOrganizationObjectStorageObjectStorageClusterWithResponse(
			ctx,
			&core.GetOrganizationObjectStorageObjectStorageClusterParams{
				OrganizationSubDomain:      &m.confOrganization,
				ObjectStorageClusterRegion: &region,
			},
		)
	body := ""
	if res != nil {
		body = string(res.Body)
	}
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, fmt.Errorf("%w: %s", core.ErrNotFound, body)
		}
		return nil, fmt.Errorf("%w: %s", err, body)
	}
	if res.JSON404 != nil {
		return nil, fmt.Errorf("%w: %s", core.ErrNotFound, body)
	}
	if res.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response (%d): %s",
			res.StatusCode(), body)
	}
	return &res.JSON200.ObjectStorageAccount, nil
}

// createObjectStorageAccount POSTs to create the object storage account for
// the given region. Does not wait for provisioning to complete.
func createObjectStorageAccount(
	ctx context.Context,
	m *Meta,
	region string,
) error {
	res, err := m.Core.
		PostOrganizationObjectStorageObjectStorageClusterWithResponse(
			ctx,
			core.PostOrganizationObjectStorageObjectStorageClusterJSONRequestBody{
				ObjectStorageCluster: core.ObjectStorageClusterLookup{
					Region: &region,
				},
				Organization: core.OrganizationLookup{
					SubDomain: &m.confOrganization,
				},
			},
		)
	if err != nil {
		body := ""
		if res != nil {
			body = string(res.Body)
		}
		return fmt.Errorf("%w: %s", err, body)
	}
	if res.JSON201 == nil {
		return fmt.Errorf("unexpected create response (%d): %s",
			res.StatusCode(), string(res.Body))
	}
	return nil
}

// waitForObjectStorageAccountProvisioned polls the account until it reaches
// the `provisioned` state. A transient `failed` state during the first
// settling window is tolerated; a sustained `failed` state results in an
// error that includes the API response body for diagnostics.
func waitForObjectStorageAccountProvisioned(
	ctx context.Context,
	m *Meta,
	region string,
) (*core.ObjectStorageAccount, error) {
	const settleWindow = 15 * time.Second

	var (
		latest    *core.ObjectStorageAccount
		firstSeen = time.Now()
	)

	waiter := &retry.StateChangeConf{
		Pending: []string{
			string(core.ObjectStorageAccountProvisioningStateEnumProvisioning),
			// `failed` is treated as pending while we're inside the settle
			// window — the API briefly reports failed during initial
			// provisioning transitions.
			string(core.ObjectStorageAccountProvisioningStateEnumFailed),
		},
		Target: []string{
			string(core.ObjectStorageAccountProvisioningStateEnumProvisioned),
		},
		Refresh: func() (interface{}, string, error) {
			acct, err := getObjectStorageAccount(ctx, m, region)
			if err != nil {
				return nil, "", err
			}
			latest = acct

			state := deref(acct.ProvisioningState)

			if state == core.ObjectStorageAccountProvisioningStateEnumFailed &&
				time.Since(firstSeen) > settleWindow {
				return acct, string(state), fmt.Errorf(
					"object storage account provisioning failed "+
						"for region %q after %s — contact Katapult support",
					region, settleWindow,
				)
			}

			return acct, string(state), nil
		},
		Timeout:                   5 * time.Minute,
		Delay:                     2 * time.Second,
		MinTimeout:                5 * time.Second,
		ContinuousTargetOccurence: 1,
	}

	if _, err := waiter.WaitForStateContext(ctx); err != nil {
		return latest, err
	}

	return latest, nil
}

// preflightObjectStorageAccountDelete returns an error describing why the
// account cannot be deleted, if any buckets or access keys still exist in the
// region. Bucket names cannot be enumerated (no list endpoint exists), so for
// buckets only a count is reported.
func preflightObjectStorageAccountDelete(
	ctx context.Context,
	m *Meta,
	region string,
) error {
	acct, err := getObjectStorageAccount(ctx, m, region)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil
		}
		return err
	}

	bucketCount := 0
	if acct.BucketCount != nil {
		bucketCount = *acct.BucketCount
	}

	keyNames, err := listObjectStorageAccessKeyNamesInRegion(ctx, m, region)
	if err != nil {
		return fmt.Errorf(
			"failed to list access keys for preflight check: %w", err,
		)
	}

	if bucketCount == 0 && len(keyNames) == 0 {
		return nil
	}

	var b strings.Builder
	fmt.Fprintf(&b,
		"cannot delete object storage account for region %q: "+
			"resources still exist.\n", region,
	)
	if bucketCount > 0 {
		fmt.Fprintf(&b,
			"  Buckets: %d still present "+
				"(the Katapult API does not expose a list endpoint; "+
				"see the Katapult dashboard for names)\n",
			bucketCount,
		)
	}
	if len(keyNames) > 0 {
		fmt.Fprintf(&b, "  Access keys: %s\n",
			strings.Join(keyNames, ", "),
		)
	}
	b.WriteString(
		"Delete these (Terraform-managed or not) before destroying the " +
			"account.",
	)
	return errors.New(b.String())
}

// listObjectStorageAccessKeyNamesInRegion returns "name (id)" strings for
// every access key in the organization scoped to the given region.
func listObjectStorageAccessKeyNamesInRegion(
	ctx context.Context,
	m *Meta,
	region string,
) ([]string, error) {
	var (
		names []string
		page  = 1
	)

	for {
		perPage := 100
		res, err := m.Core.
			GetOrganizationObjectStorageAccessKeysWithResponse(
				ctx,
				&core.GetOrganizationObjectStorageAccessKeysParams{
					OrganizationSubDomain: &m.confOrganization,
					Page:                  &page,
					PerPage:               &perPage,
				},
			)
		if err != nil {
			return nil, err
		}
		if res.JSON200 == nil {
			return nil, fmt.Errorf("unexpected list-keys response (%d): %s",
				res.StatusCode(), string(res.Body))
		}

		for i := range res.JSON200.ObjectStorageAccessKeys {
			k := res.JSON200.ObjectStorageAccessKeys[i]
			if k.Region == nil || *k.Region != region {
				continue
			}
			name := deref(k.Name)
			id := deref(k.Id)
			names = append(names, fmt.Sprintf("%s (%s)", name, id))
		}

		totalPages, _ := res.JSON200.Pagination.TotalPages.Get()
		if totalPages == 0 || page >= totalPages {
			break
		}
		page++
	}

	sort.Strings(names)
	return names, nil
}

func deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}
