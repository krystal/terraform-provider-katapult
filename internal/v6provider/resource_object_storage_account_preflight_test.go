package v6provider

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestObjectStorageAccountDeletePreflightBlocksForBuckets(t *testing.T) {
	meta := newObjectStoragePreflightTestMeta(t, 2, map[int]string{
		1: objectStorageAccessKeysPage(1, 0, ""),
	})

	err := preflightObjectStorageAccountDelete(
		context.Background(), meta, "uk-lon-1",
	)

	require.ErrorContains(t, err, `region "uk-lon-1"`)
	require.ErrorContains(t, err, "Buckets: 2 still present")
}

func TestObjectStorageAccountDeletePreflightFiltersKeysByRegion(
	t *testing.T,
) {
	meta := newObjectStoragePreflightTestMeta(t, 0, map[int]string{
		1: objectStorageAccessKeysPage(1, 1, `
			{"id":"objkey_target","name":"target","region":"uk-lon-1"},
			{"id":"objkey_other","name":"other","region":"future-region"},
			{"id":"objkey_unscoped","name":"unscoped"}`),
	})

	err := preflightObjectStorageAccountDelete(
		context.Background(), meta, "uk-lon-1",
	)

	require.ErrorContains(t, err, "Access keys: target (objkey_target)")
	require.NotContains(t, err.Error(), "other")
	require.NotContains(t, err.Error(), "unscoped")
}

func TestObjectStorageAccountDeletePreflightPaginatesAccessKeys(
	t *testing.T,
) {
	meta := newObjectStoragePreflightTestMeta(t, 0, map[int]string{
		1: objectStorageAccessKeysPage(1, 2,
			`{"id":"objkey_zulu","name":"zulu","region":"uk-lon-1"}`),
		2: objectStorageAccessKeysPage(2, 2,
			`{"id":"objkey_alpha","name":"alpha","region":"uk-lon-1"}`),
	})

	err := preflightObjectStorageAccountDelete(
		context.Background(), meta, "uk-lon-1",
	)

	require.ErrorContains(t, err,
		"Access keys: alpha (objkey_alpha), zulu (objkey_zulu)")
}

func TestObjectStorageAccountDeletePreflightPropagatesListError(
	t *testing.T,
) {
	meta := newObjectStorageFailureTestMeta(t, http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch req.URL.Path {
			case "/core/v1/organizations/organization/object_storage/" +
				"object_storage_cluster":
				_, _ = w.Write([]byte(`{
					"object_storage_account": {
						"region": "uk-lon-1",
						"bucket_count": 0
					}
				}`))
			case "/core/v1/organizations/organization/object_storage/access_keys":
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"error":"injected list failure"}`))
			default:
				http.Error(w, "unexpected request", http.StatusInternalServerError)
			}
		},
	))

	err := preflightObjectStorageAccountDelete(
		context.Background(), meta, "uk-lon-1",
	)

	require.ErrorContains(t, err,
		"failed to list access keys for preflight check")
	require.ErrorContains(t, err, "giving up after 1 attempt")
}

func newObjectStoragePreflightTestMeta(
	t *testing.T,
	bucketCount int,
	accessKeyPages map[int]string,
) *Meta {
	t.Helper()

	return newObjectStorageFailureTestMeta(t, http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch req.URL.Path {
			case "/core/v1/organizations/organization/object_storage/" +
				"object_storage_cluster":
				require.Equal(t, "uk-lon-1", req.URL.Query().Get(
					"object_storage_cluster[region]",
				))
				_, _ = fmt.Fprintf(w, `{
					"object_storage_account": {
						"region": "uk-lon-1",
						"bucket_count": %d
					}
				}`, bucketCount)
			case "/core/v1/organizations/organization/object_storage/access_keys":
				page, err := strconv.Atoi(req.URL.Query().Get("page"))
				require.NoError(t, err)
				body, ok := accessKeyPages[page]
				require.True(t, ok, "unexpected access-key page %d", page)
				_, _ = w.Write([]byte(body))
			default:
				http.Error(w, "unexpected request", http.StatusInternalServerError)
			}
		},
	))
}

func objectStorageAccessKeysPage(
	currentPage int,
	totalPages int,
	keys string,
) string {
	return fmt.Sprintf(`{
		"pagination": {
			"current_page": %d,
			"total_pages": %d,
			"per_page": 100,
			"large_set": false
		},
		"object_storage_access_keys": [%s]
	}`, currentPage, totalPages, keys)
}
