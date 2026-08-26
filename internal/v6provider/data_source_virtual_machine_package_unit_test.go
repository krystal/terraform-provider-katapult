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

func TestVirtualMachinePackageDataSourceModelNullables(t *testing.T) {
	t.Parallel()

	model := virtualMachinePackageDataSourceModel(&core.VirtualMachinePackage{
		Id:      ptr("vmpkg_test"),
		Privacy: ptr(core.Public),
	})
	assert.Equal(t, types.StringValue("vmpkg_test"), model.ID)
	assert.Equal(t, types.StringValue("public"), model.Privacy)
	assert.True(t, model.CPUCores.IsNull())
	assert.True(t, model.IPv4Addresses.IsNull())
	assert.True(t, model.MemoryInGB.IsNull())
	assert.True(t, model.StorageInGB.IsNull())
}

func TestFetchAllVirtualMachinePackagesPaginationAndErrors(t *testing.T) {
	t.Parallel()

	t.Run("fetches the final page", func(t *testing.T) {
		t.Parallel()
		var requests atomic.Int32
		client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			if r.URL.Path != "/virtual_machine_packages" ||
				r.URL.Query().Get("per_page") != "100" {
				http.NotFound(w, r)
				return
			}
			switch r.URL.Query().Get("page") {
			case "1":
				writeTestJSON(w, http.StatusOK, `{
					"virtual_machine_packages":[{"id":"vmpkg_a"}],
					"pagination":{"total_pages":2}
				}`)
			case "2":
				writeTestJSON(w, http.StatusOK, `{
					"virtual_machine_packages":[{"id":"vmpkg_b"}],
					"pagination":{"total_pages":2}
				}`)
			default:
				http.Error(w, "unexpected page", http.StatusNotFound)
			}
		})

		packages, err := fetchAllVirtualMachinePackages(
			context.Background(), &Meta{Core: client},
		)
		require.NoError(t, err)
		require.Len(t, packages, 2)
		assert.Equal(t, "vmpkg_b", *packages[1].Id)
		assert.Equal(t, int32(2), requests.Load())
	})

	t.Run("known empty", func(t *testing.T) {
		t.Parallel()
		client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			writeTestJSON(w, http.StatusOK, `{
				"virtual_machine_packages":[],
				"pagination":{"total_pages":1}
			}`)
		})
		packages, err := fetchAllVirtualMachinePackages(
			context.Background(), &Meta{Core: client},
		)
		require.NoError(t, err)
		assert.NotNil(t, packages)
		assert.Empty(t, packages)
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
			body:   `{"error":{"code":"broken","description":"package listing failed"}}`,
			want:   "broken: package listing failed",
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
			_, err := fetchAllVirtualMachinePackages(
				context.Background(), &Meta{Core: client},
			)
			require.ErrorContains(t, err, test.want)
		})
	}
}
