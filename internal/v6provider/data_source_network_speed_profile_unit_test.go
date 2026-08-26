package v6provider

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	core "github.com/krystal/go-katapult/next/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindNetworkSpeedProfileSelectorsAndNotFound(t *testing.T) {
	t.Parallel()

	profiles := []core.NetworkSpeedProfile{
		{Id: ptr("nsp_a"), Permalink: ptr("fast")},
		{Id: ptr("nsp_b"), Permalink: ptr("unlimited")},
	}

	profile, selector, value := findNetworkSpeedProfile(
		profiles, types.StringValue("nsp_b"), types.StringValue("ignored"),
	)
	require.NotNil(t, profile)
	assert.Equal(t, "nsp_b", *profile.Id)
	assert.Equal(t, "id", selector)
	assert.Equal(t, "nsp_b", value)

	profile, selector, value = findNetworkSpeedProfile(
		profiles, types.StringNull(), types.StringValue("fast"),
	)
	require.NotNil(t, profile)
	assert.Equal(t, "nsp_a", *profile.Id)
	assert.Equal(t, "permalink", selector)
	assert.Equal(t, "fast", value)

	profile, selector, value = findNetworkSpeedProfile(
		profiles, types.StringNull(), types.StringValue("missing"),
	)
	assert.Nil(t, profile)
	assert.Equal(t, "permalink", selector)
	assert.Equal(t, "missing", value)
}

func TestNetworkSpeedProfileDataSourceModelNullables(t *testing.T) {
	t.Parallel()

	profile := core.NetworkSpeedProfile{
		Id:        ptr("nsp_test"),
		Name:      ptr("Test"),
		Permalink: ptr("test"),
	}
	profile.UploadSpeedInMbit.Set(500)
	profile.DownloadSpeedInMbit.SetNull()
	model := networkSpeedProfileDataSourceModel(&profile)
	assert.Equal(t, types.Int64Value(500), model.UploadSpeed)
	assert.True(t, model.DownloadSpeed.IsNull())
}

func TestFetchAllOrganizationNetworkSpeedProfilesPaginationAndErrors(t *testing.T) {
	t.Parallel()

	t.Run("fetches the final page", func(t *testing.T) {
		t.Parallel()
		var requests atomic.Int32
		client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			if r.URL.Path != "/organizations/organization/network_speed_profiles" ||
				r.URL.Query().Get("organization[sub_domain]") != "test-org" ||
				r.URL.Query().Get("per_page") != "200" {
				http.NotFound(w, r)
				return
			}
			switch r.URL.Query().Get("page") {
			case "1":
				writeTestJSON(w, http.StatusOK, `{
					"network_speed_profiles":[{"id":"nsp_a"}],
					"pagination":{"total_pages":2}
				}`)
			case "2":
				writeTestJSON(w, http.StatusOK, `{
					"network_speed_profiles":[{"id":"nsp_b"}],
					"pagination":{"total_pages":2}
				}`)
			default:
				http.Error(w, "unexpected page", http.StatusNotFound)
			}
		})

		profiles, err := fetchAllOrganizationNetworkSpeedProfiles(
			context.Background(),
			&Meta{Core: client, confOrganization: "test-org"},
		)
		require.NoError(t, err)
		require.Len(t, profiles, 2)
		assert.Equal(t, "nsp_b", *profiles[1].Id)
		assert.Equal(t, int32(2), requests.Load())
	})

	t.Run("known empty", func(t *testing.T) {
		t.Parallel()
		client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			writeTestJSON(w, http.StatusOK, `{
				"network_speed_profiles":[],
				"pagination":{"total_pages":1}
			}`)
		})
		profiles, err := fetchAllOrganizationNetworkSpeedProfiles(
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
			_, err := fetchAllOrganizationNetworkSpeedProfiles(
				context.Background(), &Meta{Core: client},
			)
			require.ErrorContains(t, err, test.want)
		})
	}
}
