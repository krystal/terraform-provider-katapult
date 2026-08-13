package v6provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/krystal/go-katapult/next/core"
)

const (
	objectStorageAccountTrashIDPrivateKey = "object_storage_account_trash_id"
	objectStorageNoResponseBody           = "<no response>"
	objectStorageRegionAttributeName      = "region"
)

type (
	ObjectStorageAccountResource struct {
		M *Meta
	}

	ObjectStorageAccountResourceModel struct {
		Region            types.String `tfsdk:"region"`
		AdoptExisting     types.Bool   `tfsdk:"adopt_existing"`
		ProvisioningState types.String `tfsdk:"provisioning_state"`
	}
)

var objectStorageAccountMarkdownDesc = strings.TrimSpace(`
Manages the lifecycle of an object storage account for an organization in a given region.
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
			objectStorageRegionAttributeName: schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Object storage region. Currently the " +
					"only available region is `uk-lon-1`. Changing this " +
					"forces replacement.",
				Validators: []validator.String{
					stringValidatorNotEmpty(),
				},
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
					"Defaults to `false`. This is only used during create; " +
					"changing it later updates Terraform state without " +
					"changing the remote account.",
				Default: booldefault.StaticBool(false),
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

	plan.Region = types.StringValue(region)
	plan.AdoptExisting = types.BoolValue(adopt)
	plan.ProvisioningState = types.StringNull()
	if existing != nil && existing.ProvisioningState != nil {
		plan.ProvisioningState = types.StringValue(
			string(*existing.ProvisioningState),
		)
	}

	// Persist recoverable identity and configuration immediately after the
	// account exists remotely. Terraform retains this state when a later
	// provisioning diagnostic fails, allowing refresh or destroy to recover.
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
	if resp.Diagnostics.HasError() {
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

	acct, err := getObjectStorageAccount(ctx, r.M, region)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			privateTrashID, privateDiags := req.Private.GetKey(
				ctx, objectStorageAccountTrashIDPrivateKey,
			)
			resp.Diagnostics.Append(privateDiags...)
			if resp.Diagnostics.HasError() {
				return
			}

			trashID, decodeErr := decodeObjectStorageAccountTrashID(
				privateTrashID,
			)
			if decodeErr != nil {
				resp.Diagnostics.AddError(
					"Invalid Object Storage Account Private State",
					decodeErr.Error(),
				)
				return
			}
			if trashID != "" {
				// The remote account is already deleted, but Terraform must
				// retain state until Delete can resume the pending trash purge.
				resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
				return
			}

			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Object Storage Account Read Error",
			err.Error(),
		)
		return
	}

	state.Region = types.StringValue(region)
	state.ProvisioningState = types.StringValue(
		string(deref(acct.ProvisioningState)),
	)
	// adopt_existing is a Create-time-only knob; it doesn't reflect any
	// server-side property and is left untouched by Read.
	resp.Diagnostics.Append(resp.Private.SetKey(
		ctx, objectStorageAccountTrashIDPrivateKey, nil,
	)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *ObjectStorageAccountResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan, state ObjectStorageAccountResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// adopt_existing has no server-side representation. Its only purpose is
	// controlling Create behavior, so an update is deliberately state-only.
	// Preserve computed values because they may be unknown in the update plan.
	plan.ProvisioningState = state.ProvisioningState
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
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

	privateTrashID, privateDiags := req.Private.GetKey(
		ctx, objectStorageAccountTrashIDPrivateKey,
	)
	resp.Diagnostics.Append(privateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	trashID, decodeErr := decodeObjectStorageAccountTrashID(privateTrashID)
	if decodeErr != nil {
		resp.Diagnostics.AddError(
			"Invalid Object Storage Account Private State",
			decodeErr.Error(),
		)
		return
	}
	if trashID != "" {
		if r.M.SkipTrashObjectPurge {
			return
		}
		if purgeErr := purgeTrashObject(
			ctx, r.M, 5*time.Minute, core.TrashObject{Id: &trashID},
		); purgeErr != nil && !errors.Is(purgeErr, core.ErrNotFound) {
			resp.Diagnostics.AddError(
				"Failed to purge object storage account from trash.",
				purgeErr.Error(),
			)
			return
		}
		resp.Diagnostics.Append(resp.Private.SetKey(
			ctx, objectStorageAccountTrashIDPrivateKey, nil,
		)...)
		return
	}

	region := state.Region.ValueString()

	// Preflight: refuse to delete the account if any buckets or access keys
	// still exist in this region — managed by Terraform or not.
	if preflightErr := preflightObjectStorageAccountDelete(
		ctx, r.M, region,
	); preflightErr != nil {
		resp.Diagnostics.AddError(
			"Object Storage Account Delete Blocked",
			preflightErr.Error(),
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

	if delRes == nil || delRes.JSON200 == nil ||
		delRes.JSON200.TrashObject.Id == nil {
		// Nothing to purge — either the API didn't move the account to
		// trash, or the response shape is unexpected. Don't fail the
		// destroy on this.
		return
	}

	trashID = *delRes.JSON200.TrashObject.Id
	encodedTrashID, err := encodeObjectStorageAccountTrashID(trashID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to encode object storage account trash ID.",
			err.Error(),
		)
		return
	}
	resp.Diagnostics.Append(resp.Private.SetKey(
		ctx, objectStorageAccountTrashIDPrivateKey, encodedTrashID,
	)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := purgeTrashObject(
		ctx, r.M, 5*time.Minute, core.TrashObject{Id: &trashID},
	); err != nil {
		resp.Diagnostics.AddError(
			"Failed to purge object storage account from trash.",
			err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(resp.Private.SetKey(
		ctx, objectStorageAccountTrashIDPrivateKey, nil,
	)...)
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
			"Expected import ID to be an object storage region, e.g. uk-lon-1.",
		)
		return
	}

	resp.Diagnostics.Append(
		resp.State.SetAttribute(
			ctx, path.Root(objectStorageRegionAttributeName), region,
		)...,
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
	if res != nil && res.JSON404 != nil {
		return nil, fmt.Errorf("%w: %s", core.ErrNotFound, body)
	}
	if res == nil || res.JSON200 == nil {
		status := 0
		if res != nil {
			status = res.StatusCode()
		}
		return nil, fmt.Errorf("unexpected response (%d): %s",
			status, body)
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
	if res == nil || res.JSON201 == nil {
		status := 0
		body := ""
		if res != nil {
			status = res.StatusCode()
			body = string(res.Body)
		}
		return fmt.Errorf("unexpected create response (%d): %s",
			status, body)
	}
	return nil
}

// waitForObjectStorageAccountProvisioned polls the account until it reaches
// the `provisioned` state. A transient `failed` state is tolerated for a
// settling window; a sustained `failed` state results in an error that includes
// the API response body for diagnostics.
func waitForObjectStorageAccountProvisioned(
	ctx context.Context,
	m *Meta,
	region string,
) (*core.ObjectStorageAccount, error) {
	settleWindow := 15 * time.Second
	if replayPollInterval := m.stateChangePollInterval(); replayPollInterval > 0 {
		// Replay advances recorded states per poll rather than over wall-clock
		// seconds. Preserve a bounded settling window without waiting 15 seconds.
		settleWindow = 15 * replayPollInterval
	}

	var failedSince time.Time

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
			state := deref(acct.ProvisioningState)

			if state == core.ObjectStorageAccountProvisioningStateEnumFailed {
				if failedSince.IsZero() {
					failedSince = time.Now()
				}
				if time.Since(failedSince) > settleWindow {
					return acct, string(state), fmt.Errorf(
						"object storage account provisioning failed "+
							"for region %q after %s — contact Katapult support",
						region, settleWindow,
					)
				}
			} else {
				failedSince = time.Time{}
			}

			return acct, string(state), nil
		},
		Timeout:                   5 * time.Minute,
		Delay:                     m.stateChangeDelay(2 * time.Second),
		MinTimeout:                m.stateChangeDelay(5 * time.Second),
		PollInterval:              m.stateChangePollInterval(),
		ContinuousTargetOccurence: 1,
	}

	result, err := waiter.WaitForStateContext(ctx)
	if err != nil {
		return nil, err
	}

	acct, ok := result.(*core.ObjectStorageAccount)
	if !ok || acct == nil {
		return nil, fmt.Errorf(
			"unexpected object storage account waiter result: %T",
			result,
		)
	}

	return acct, nil
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

	if acct.BucketCount == nil {
		return errors.New(
			"cannot determine whether object storage account deletion is safe: " +
				"account response is missing bucket_count",
		)
	}
	bucketCount := *acct.BucketCount

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
		if res == nil || res.JSON200 == nil {
			status := 0
			body := ""
			if res != nil {
				status = res.StatusCode()
				body = string(res.Body)
			}
			return nil, fmt.Errorf("unexpected list-keys response (%d): %s",
				status, body)
		}

		for i := range res.JSON200.ObjectStorageAccessKeys {
			k := res.JSON200.ObjectStorageAccessKeys[i]
			if k.Region == nil {
				return nil, fmt.Errorf(
					"cannot determine access key region during delete preflight: "+
						"access key %s is missing region",
					deref(k.Id),
				)
			}
			if *k.Region != region {
				continue
			}
			name := deref(k.Name)
			id := deref(k.Id)
			names = append(names, fmt.Sprintf("%s (%s)", name, id))
		}

		morePages, paginationErr := objectStorageAccessKeysHaveMorePages(
			res.JSON200.Pagination,
			page,
			len(res.JSON200.ObjectStorageAccessKeys),
		)
		if paginationErr != nil {
			return nil, paginationErr
		}
		if !morePages {
			break
		}
		page++
	}

	sort.Strings(names)
	return names, nil
}

func objectStorageAccessKeysHaveMorePages(
	pagination core.PaginationObject,
	requestedPage int,
	itemCount int,
) (bool, error) {
	if pagination.CurrentPage == nil {
		return false, fmt.Errorf(
			"invalid access-key pagination on requested page %d: "+
				"missing current_page", requestedPage,
		)
	}
	if *pagination.CurrentPage != requestedPage {
		return false, fmt.Errorf(
			"invalid access-key pagination on requested page %d: "+
				"response current_page is %d",
			requestedPage, *pagination.CurrentPage,
		)
	}

	if pagination.TotalPages.IsSpecified() &&
		!pagination.TotalPages.IsNull() {
		totalPages := pagination.TotalPages.MustGet()
		if totalPages == 0 && requestedPage == 1 && itemCount == 0 {
			return false, nil
		}
		if totalPages < 1 || totalPages < requestedPage {
			return false, fmt.Errorf(
				"invalid access-key pagination on page %d: total_pages is %d",
				requestedPage, totalPages,
			)
		}

		return requestedPage < totalPages, nil
	}

	if pagination.LargeSet == nil || !*pagination.LargeSet {
		return false, fmt.Errorf(
			"indeterminate access-key pagination on page %d: "+
				"total_pages is missing or null and large_set is not true",
			requestedPage,
		)
	}
	if pagination.PerPage == nil || *pagination.PerPage < 1 {
		return false, fmt.Errorf(
			"invalid access-key pagination on large-set page %d: "+
				"missing or invalid per_page", requestedPage,
		)
	}

	return itemCount >= *pagination.PerPage, nil
}

func deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

func encodeObjectStorageAccountTrashID(trashID string) ([]byte, error) {
	return json.Marshal(trashID)
}

func decodeObjectStorageAccountTrashID(data []byte) (string, error) {
	if len(data) == 0 {
		return "", nil
	}

	var trashID string
	if err := json.Unmarshal(data, &trashID); err != nil {
		return "", fmt.Errorf("decode trash ID: %w", err)
	}
	if trashID == "" {
		return "", errors.New("decoded trash ID is empty")
	}

	return trashID, nil
}
