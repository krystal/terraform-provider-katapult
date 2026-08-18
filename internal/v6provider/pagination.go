package v6provider

import core "github.com/krystal/go-katapult/next/core"

func paginationHasNext(
	pagination core.PaginationObject,
	page int,
	itemCount int,
	fallbackPageSize int,
) bool {
	if pagination.TotalPages.IsSpecified() && !pagination.TotalPages.IsNull() {
		if totalPages, err := pagination.TotalPages.Get(); err == nil {
			return page < totalPages
		}
	}

	pageSize := fallbackPageSize
	if pagination.PerPage != nil && *pagination.PerPage > 0 {
		pageSize = *pagination.PerPage
	}

	return itemCount >= pageSize
}
