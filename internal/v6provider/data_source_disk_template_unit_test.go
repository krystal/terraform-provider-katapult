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

func TestDiskTemplateDataSourceModelsHandleNullableRelationships(t *testing.T) {
	t.Parallel()

	direct := core.GetDiskTemplate200ResponseDiskTemplate{
		Id:        ptr("dtpl_direct"),
		Name:      ptr("Direct"),
		Permalink: ptr("templates/direct"),
		Universal: ptr(true),
	}
	direct.Description.Set("Direct description")
	direct.LatestVersion.Set(core.GetDiskTemplatePartLatestVersion{Id: ptr("dtplv_3")})
	direct.OperatingSystem.Set(core.OperatingSystem{Name: ptr("Linux")})
	directModel := diskTemplateDataSourceModelFromGet(&direct)
	assert.Equal(t, types.StringValue("Direct description"), directModel.Description)
	assert.True(t, directModel.TemplateVersion.IsNull())
	assert.Equal(t, types.StringValue("Linux"), directModel.OSFamily)

	listed := core.GetOrganizationDiskTemplates200ResponseDiskTemplates{
		Id: ptr("dtpl_listed"),
	}
	listed.Description.SetNull()
	listed.LatestVersion.SetNull()
	listed.OperatingSystem.SetNull()
	listedModel := diskTemplateDataSourceModelFromList(&listed)
	assert.Equal(t, types.StringValue(""), listedModel.Description)
	assert.True(t, listedModel.TemplateVersion.IsNull())
	assert.True(t, listedModel.OSFamily.IsNull())
}

func TestFetchDiskTemplateVersionNumber(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name        string
		contentType string
		status      int
		body        string
		want        types.Int64
		wantErr     string
	}{
		{
			name: "version number", contentType: "application/json",
			status: http.StatusOK,
			body:   `{"disk_template_version":{"id":"dtplv_3","number":3}}`,
			want:   types.Int64Value(3),
		},
		{
			name: "API error", contentType: "application/json",
			status:  http.StatusInternalServerError,
			body:    `{"error":{"code":"broken","description":"version fetch failed"}}`,
			wantErr: "broken: version fetch failed",
		},
		{
			name: "empty successful response", contentType: "text/plain",
			status:  http.StatusOK,
			wantErr: "unexpected empty response fetching disk template version dtplv_3",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/disk_template_versions/disk_template_version" ||
					r.URL.Query().Get("disk_template_version[id]") != "dtplv_3" {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", test.contentType)
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			})

			got, err := fetchDiskTemplateVersionNumber(
				context.Background(), &Meta{Core: client}, "dtplv_3",
			)
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestFetchAllOrganizationDiskTemplatesPaginationAndErrors(t *testing.T) {
	t.Parallel()

	t.Run("fetches the final page", func(t *testing.T) {
		t.Parallel()
		var requests atomic.Int32
		client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			if r.URL.Path != "/organizations/organization/disk_templates" ||
				r.URL.Query().Get("organization[sub_domain]") != "test-org" ||
				r.URL.Query().Get("include_universal") != "true" ||
				r.URL.Query().Get("per_page") != "100" {
				http.NotFound(w, r)
				return
			}
			switch r.URL.Query().Get("page") {
			case "1":
				writeTestJSON(w, http.StatusOK, `{
					"disk_templates":[{"id":"dtpl_a"}],
					"pagination":{"total_pages":2}
				}`)
			case "2":
				writeTestJSON(w, http.StatusOK, `{
					"disk_templates":[{"id":"dtpl_b"}],
					"pagination":{"total_pages":2}
				}`)
			default:
				http.Error(w, "unexpected page", http.StatusNotFound)
			}
		})

		templates, err := fetchAllOrganizationDiskTemplates(
			context.Background(),
			&Meta{Core: client, confOrganization: "test-org"},
			true,
		)
		require.NoError(t, err)
		require.Len(t, templates, 2)
		assert.Equal(t, "dtpl_b", *templates[1].Id)
		assert.Equal(t, int32(2), requests.Load())
	})

	t.Run("known empty", func(t *testing.T) {
		t.Parallel()
		client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			writeTestJSON(w, http.StatusOK, `{
				"disk_templates":[],
				"pagination":{"total_pages":1}
			}`)
		})
		templates, err := fetchAllOrganizationDiskTemplates(
			context.Background(), &Meta{Core: client}, false,
		)
		require.NoError(t, err)
		assert.NotNil(t, templates)
		assert.Empty(t, templates)
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
			body:   `{"error":{"code":"broken","description":"template listing failed"}}`,
			want:   "broken: template listing failed",
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
			_, err := fetchAllOrganizationDiskTemplates(
				context.Background(), &Meta{Core: client}, false,
			)
			require.ErrorContains(t, err, test.want)
		})
	}
}
