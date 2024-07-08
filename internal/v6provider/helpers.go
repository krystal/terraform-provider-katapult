package v6provider

import (
	"context"
	"errors"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/krystal/go-katapult/next/core"
)

func isErrNotFoundOrInTrash(err error, res *core.ObjectInTrashResponse) bool {
	return errors.Is(err, core.ErrNotFound) ||
		(res != nil && *res.Code == core.ObjectInTrashEnumObjectInTrash)
}

func purgeTrashObjectByObjectID(
	ctx context.Context,
	m *Meta,
	timeout time.Duration,
	objectID string,
) error {
	return purgeTrashObject(
		ctx, m, timeout, core.TrashObject{ObjectId: &objectID},
	)
}

func purgeTrashObject(
	ctx context.Context,
	m *Meta,
	timeout time.Duration,
	trashObject core.TrashObject,
) error {
	_, err := m.Core.DeleteTrashObjectWithResponse(ctx,
		core.DeleteTrashObjectJSONRequestBody{
			TrashObject: core.TrashObjectLookup{
				Id:       trashObject.Id,
				ObjectId: trashObject.ObjectId,
			},
		})
	if err != nil {
		return err
	}

	err = waitForTrashObjectNotFound(ctx, m, timeout, trashObject)

	return err
}

func waitForTrashObjectNotFound(
	ctx context.Context,
	m *Meta,
	timeout time.Duration,
	trashObject core.TrashObject,
) error {
	waiter := &retry.StateChangeConf{
		Pending: []string{"exists"},
		Target:  []string{"not_found"},
		Refresh: func() (interface{}, string, error) {
			_, e := m.Core.GetTrashObjectWithResponse(ctx,
				&core.GetTrashObjectParams{
					TrashObjectId:       trashObject.Id,
					TrashObjectObjectId: trashObject.ObjectId,
				})
			if e != nil && errors.Is(e, core.ErrNotFound) {
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
