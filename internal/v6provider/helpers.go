package v6provider

import (
	"context"
	"errors"
	"time"

	"github.com/krystal/go-katapult"
	"github.com/krystal/go-katapult/core"
)

func isErrNotFoundOrInTrash(err error) bool {
	return errors.Is(err, katapult.ErrNotFound) ||
		errors.Is(err, core.ErrObjectInTrash)
}

func purgeTrashObjectByObjectID(
	ctx context.Context,
	m *Meta,
	timeout time.Duration,
	objectID string,
) error {
	return purgeTrashObject(
		ctx, m, timeout, core.TrashObjectRef{ObjectID: objectID},
	)
}

func purgeTrashObject(
	ctx context.Context,
	m *Meta,
	timeout time.Duration,
	ref core.TrashObjectRef,
) error {
	_, _, err := m.Core.TrashObjects.Purge(ctx, ref)
	if err != nil {
		if errors.Is(err, katapult.ErrNotFound) {
			return nil
		}

		return err
	}

	err = waitForTrashObjectNotFound(ctx, m, timeout, ref)

	return err
}

func waitForTrashObjectNotFound(
	ctx context.Context,
	m *Meta,
	timeout time.Duration,
	ref core.TrashObjectRef,
) error {
	waiter := &Waiter{
		Pending: []string{"exists"},
		Target:  []string{"not_found"},
		Refresh: func() (interface{}, string, error) {
			_, _, e := m.Core.TrashObjects.Get(ctx, ref)
			if e != nil && errors.Is(e, katapult.ErrNotFound) {
				return 1, "not_found", nil
			}

			return nil, "exists", nil
		},
		Timeout:                   timeout,
		Delay:                     1 * time.Second,
		MinTimeout:                5 * time.Second,
		ContinuousTargetOccurence: 1,
	}

	_, err := waiter.WaitForStateContext(ctx)

	return err
}
