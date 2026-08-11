package v6provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/krystal/go-katapult/next/core"
	"github.com/stretchr/testify/require"
)

func TestObjectStorageAccountDeletePreflightBlocksForBuckets(t *testing.T) {
	meta := newObjectStoragePreflightTestMeta(t, 2, map[int]string{
		1: objectStorageAccessKeysPage(1, 1, ""),
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
			{"id":"objkey_other","name":"other","region":"future-region"}`),
	})

	err := preflightObjectStorageAccountDelete(
		context.Background(), meta, "uk-lon-1",
	)

	require.ErrorContains(t, err, "Access keys: target (objkey_target)")
	require.NotContains(t, err.Error(), "other")
}

func TestObjectStorageAccountDeletePreflightRejectsMissingBucketCount(
	t *testing.T,
) {
	var listCalls int
	meta := newObjectStorageFailureTestMeta(t, http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch req.URL.Path {
			case "/core/v1/organizations/organization/object_storage/" +
				"object_storage_cluster":
				_, _ = w.Write([]byte(`{
					"object_storage_account": {"region": "uk-lon-1"}
				}`))
			case "/core/v1/organizations/organization/object_storage/access_keys":
				listCalls++
				http.Error(w, "must not list keys", http.StatusInternalServerError)
			default:
				http.Error(w, "unexpected request", http.StatusInternalServerError)
			}
		},
	))

	err := preflightObjectStorageAccountDelete(
		context.Background(), meta, "uk-lon-1",
	)

	require.ErrorContains(t, err, "missing bucket_count")
	require.Zero(t, listCalls)
}

func TestObjectStorageAccountDeletePreflightRejectsAccessKeyWithoutRegion(
	t *testing.T,
) {
	meta := newObjectStoragePreflightTestMeta(t, 0, map[int]string{
		1: objectStorageAccessKeysPage(1, 1,
			`{"id":"objkey_unscoped","name":"unscoped"}`),
	})

	err := preflightObjectStorageAccountDelete(
		context.Background(), meta, "uk-lon-1",
	)

	require.ErrorContains(t, err, "access key objkey_unscoped")
	require.ErrorContains(t, err, "missing region")
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

func TestObjectStorageAccountDeletePreflightAcceptsEmptyZeroPagePagination(
	t *testing.T,
) {
	meta := newObjectStoragePreflightTestMeta(t, 0, map[int]string{
		1: `{
			"pagination": {
				"current_page": 1,
				"total": 0,
				"total_pages": 0,
				"per_page": 100,
				"large_set": false
			},
			"object_storage_access_keys": []
		}`,
	})

	err := preflightObjectStorageAccountDelete(
		context.Background(), meta, "uk-lon-1",
	)

	require.NoError(t, err)
}

func TestObjectStorageAccessKeysPaginationRejectsInconsistentZeroPage(
	t *testing.T,
) {
	testCases := map[string]struct {
		requestedPage int
		itemCount     int
		pagination    string
	}{
		"items on first page": {
			requestedPage: 1,
			itemCount:     1,
			pagination: `{
				"current_page": 1,
				"total": 0,
				"total_pages": 0,
				"per_page": 100,
				"large_set": false
			}`,
		},
		"later requested page": {
			requestedPage: 2,
			itemCount:     0,
			pagination: `{
				"current_page": 2,
				"total": 0,
				"total_pages": 0,
				"per_page": 100,
				"large_set": false
			}`,
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			var pagination core.PaginationObject
			require.NoError(t, json.Unmarshal(
				[]byte(testCase.pagination), &pagination,
			))

			morePages, err := objectStorageAccessKeysHaveMorePages(
				pagination, testCase.requestedPage, testCase.itemCount,
			)

			require.ErrorContains(t, err, "total_pages is 0")
			require.False(t, morePages)
		})
	}
}

func TestObjectStorageAccountDeletePreflightPaginatesLargeSetWithoutTotals(
	t *testing.T,
) {
	requestedPages := []int{}
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
				page, err := strconv.Atoi(req.URL.Query().Get("page"))
				require.NoError(t, err)
				requestedPages = append(requestedPages, page)
				switch page {
				case 1:
					_, _ = w.Write([]byte(objectStorageLargeSetAccessKeysPage(
						1, 2, `
							{"id":"objkey_other_1","name":"other-1","region":"other"},
							{"id":"objkey_other_2","name":"other-2","region":"other"}`,
					)))
				case 2:
					_, _ = w.Write([]byte(objectStorageLargeSetAccessKeysPage(
						2, 2,
						`{"id":"objkey_other_3","name":"other-3","region":"other"}`,
					)))
				default:
					http.Error(w, "unexpected page", http.StatusInternalServerError)
				}
			default:
				http.Error(w, "unexpected request", http.StatusInternalServerError)
			}
		},
	))

	err := preflightObjectStorageAccountDelete(
		context.Background(), meta, "uk-lon-1",
	)

	require.NoError(t, err)
	require.Equal(t, []int{1, 2}, requestedPages)
}

func TestObjectStorageAccountDeletePreflightRejectsIndeterminatePagination(
	t *testing.T,
) {
	testCases := map[string]string{
		"missing total pages for non-large set": `{
			"pagination": {
				"current_page": 1,
				"per_page": 100,
				"large_set": false
			},
			"object_storage_access_keys": []
		}`,
		"null total pages for non-large set": `{
			"pagination": {
				"current_page": 1,
				"total_pages": null,
				"per_page": 100,
				"large_set": false
			},
			"object_storage_access_keys": []
		}`,
		"large set missing current page": `{
			"pagination": {
				"per_page": 100,
				"large_set": true
			},
			"object_storage_access_keys": []
		}`,
		"large set missing page size": `{
			"pagination": {
				"current_page": 1,
				"large_set": true
			},
			"object_storage_access_keys": []
		}`,
	}

	for name, page := range testCases {
		t.Run(name, func(t *testing.T) {
			meta := newObjectStoragePreflightTestMeta(t, 0, map[int]string{1: page})

			err := preflightObjectStorageAccountDelete(
				context.Background(), meta, "uk-lon-1",
			)

			require.ErrorContains(t, err, "pagination")
		})
	}
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

func objectStorageLargeSetAccessKeysPage(
	currentPage int,
	perPage int,
	keys string,
) string {
	return fmt.Sprintf(`{
		"pagination": {
			"current_page": %d,
			"per_page": %d,
			"large_set": true
		},
		"object_storage_access_keys": [%s]
	}`, currentPage, perPage, keys)
}
