package v6provider

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	core "github.com/krystal/go-katapult/next/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiskIOProfileDataSourceSchemaSelectors(t *testing.T) {
	t.Parallel()

	response := &datasource.SchemaResponse{}
	(&DiskIOProfileDataSource{}).Schema(
		context.Background(), datasource.SchemaRequest{}, response,
	)
	require.False(t, response.Diagnostics.HasError())

	id := response.Schema.Attributes["id"]
	permalink := response.Schema.Attributes["permalink"]
	assert.True(t, id.IsOptional())
	assert.True(t, id.IsComputed())
	assert.True(t, permalink.IsOptional())
	assert.True(t, permalink.IsComputed())
	assert.Len(t, (&DiskIOProfileDataSource{}).ConfigValidators(context.Background()), 1)
}

func TestFindDiskIOProfileSelectorsAndNotFound(t *testing.T) {
	t.Parallel()

	profiles := []core.DiskIOProfile{
		{Id: ptr("iop_a"), Permalink: ptr("fast")},
		{Id: ptr("iop_b"), Permalink: ptr("unlimited")},
	}

	profile, selector, value := findDiskIOProfile(
		profiles, types.StringValue("iop_b"), types.StringNull(),
	)
	require.NotNil(t, profile)
	assert.Equal(t, "iop_b", *profile.Id)
	assert.Equal(t, "id", selector)
	assert.Equal(t, "iop_b", value)

	profile, selector, value = findDiskIOProfile(
		profiles, types.StringNull(), types.StringValue("fast"),
	)
	require.NotNil(t, profile)
	assert.Equal(t, "iop_a", *profile.Id)
	assert.Equal(t, "permalink", selector)
	assert.Equal(t, "fast", value)

	profile, selector, value = findDiskIOProfile(
		profiles, types.StringNull(), types.StringValue("missing"),
	)
	assert.Nil(t, profile)
	assert.Equal(t, "permalink", selector)
	assert.Equal(t, "missing", value)
}

func TestDiskIOProfileDataSourceModelNullables(t *testing.T) {
	t.Parallel()

	profile := core.DiskIOProfile{
		Id:        ptr("iop_test"),
		Permalink: ptr("test"),
		Name:      ptr("Test"),
	}
	profile.SpeedInMb.Set(500)
	profile.Iops.SetNull()
	model := diskIOProfileDataSourceModel(&profile)
	assert.Equal(t, types.Int64Value(500), model.SpeedInMB)
	assert.True(t, model.IOPS.IsNull())

	unspecified := diskIOProfileDataSourceModel(&core.DiskIOProfile{})
	assert.True(t, unspecified.SpeedInMB.IsNull())
	assert.True(t, unspecified.IOPS.IsNull())
}

func TestFetchAllOrganizationDiskIOProfilesPaginationSortingEmptyAndErrors(t *testing.T) {
	t.Parallel()

	t.Run("pagination and sorting", func(t *testing.T) {
		t.Parallel()
		var requests atomic.Int32
		client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			require.Equal(t, "/organizations/organization/disk_io_profiles", r.URL.Path)
			require.Equal(t, "test-org", r.URL.Query().Get("organization[sub_domain]"))
			require.Equal(t, "200", r.URL.Query().Get("per_page"))
			switch r.URL.Query().Get("page") {
			case "1":
				writeTestJSON(w, http.StatusOK, `{"disk_io_profiles":[{"id":"iop_z"}],"pagination":{"total_pages":2}}`)
			case "2":
				writeTestJSON(w, http.StatusOK, `{"disk_io_profiles":[{"id":"iop_a"}],"pagination":{"total_pages":2}}`)
			default:
				t.Errorf("unexpected page %q", r.URL.Query().Get("page"))
			}
		})

		profiles, err := fetchAllOrganizationDiskIOProfiles(context.Background(), &Meta{
			Core: client, confOrganization: "test-org",
		})
		require.NoError(t, err)
		require.Len(t, profiles, 2)
		assert.Equal(t, "iop_a", *profiles[0].Id)
		assert.Equal(t, "iop_z", *profiles[1].Id)
		assert.Equal(t, int32(2), requests.Load())
	})

	t.Run("missing IDs sort last", func(t *testing.T) {
		t.Parallel()
		client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			writeTestJSON(w, http.StatusOK, `{
				"disk_io_profiles":[{"name":"missing"},{"id":"iop_a"}],
				"pagination":{"total_pages":1}
			}`)
		})
		profiles, err := fetchAllOrganizationDiskIOProfiles(
			context.Background(), &Meta{Core: client},
		)
		require.NoError(t, err)
		require.Len(t, profiles, 2)
		assert.Equal(t, "iop_a", *profiles[0].Id)
		assert.Nil(t, profiles[1].Id)
	})

	t.Run("known empty", func(t *testing.T) {
		t.Parallel()
		client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			writeTestJSON(w, http.StatusOK, `{"disk_io_profiles":[],"pagination":{"total_pages":1}}`)
		})
		profiles, err := fetchAllOrganizationDiskIOProfiles(
			context.Background(), &Meta{Core: client},
		)
		require.NoError(t, err)
		assert.NotNil(t, profiles)
		assert.Empty(t, profiles)
	})

	for _, test := range []struct {
		name        string
		contentType string
		status      int
		body        string
		want        string
	}{
		{
			name: "API error", contentType: "application/json",
			status: http.StatusInternalServerError,
			body:   `{"error":{"code":"broken","description":"profile listing failed"}}`,
			want:   "broken: profile listing failed",
		},
		{
			name: "empty successful response", contentType: "text/plain",
			status: http.StatusOK, want: "unexpected empty response",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", test.contentType)
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			})
			_, err := fetchAllOrganizationDiskIOProfiles(
				context.Background(), &Meta{Core: client},
			)
			require.ErrorContains(t, err, test.want)
		})
	}
}
