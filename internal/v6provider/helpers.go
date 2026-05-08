package v6provider

import (
	"context"
	"errors"
	"fmt"
	"slices"
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
	lookup := core.TrashObjectLookup{}
	if trashObject.Id != nil {
		lookup.Id = trashObject.Id
	} else {
		lookup.ObjectId = trashObject.ObjectId
	}

	_, err := m.Core.DeleteTrashObjectWithResponse(ctx,
		core.DeleteTrashObjectJSONRequestBody{
			TrashObject: lookup,
		})
	if err != nil {
		return err
	}

	err = waitForTrashObjectNotFound(ctx, m, timeout, trashObject)

	return err
}

func waitForTaskCompletion(
	ctx context.Context,
	m *Meta,
	timeout time.Duration,
	taskID string,
) error {
	waiter := &retry.StateChangeConf{
		Pending: []string{
			string(core.TaskStatusEnumPending),
			string(core.TaskStatusEnumRunning),
		},
		Target: []string{
			string(core.TaskStatusEnumCompleted),
		},
		Refresh: func() (interface{}, string, error) {
			res, e := m.Core.GetTaskWithResponse(ctx,
				&core.GetTaskParams{TaskId: &taskID})
			if e != nil {
				if res != nil {
					e = genericAPIError(e, res.Body)
				}
				return nil, "", e
			}

			task := res.JSON200.Task
			if task.Status == nil {
				return task, "", fmt.Errorf("task status is nil")
			}
			if *task.Status == core.TaskStatusEnumFailed {
				return task, string(*task.Status),
					fmt.Errorf("task failed")
			}

			return task, string(*task.Status), nil
		},
		Timeout:                   timeout,
		Delay:                     m.stateChangeDelay(1 * time.Second),
		MinTimeout:                m.stateChangeDelay(5 * time.Second),
		PollInterval:              m.stateChangePollInterval(),
		ContinuousTargetOccurence: 1,
	}

	_, err := waiter.WaitForStateContext(ctx)

	return err
}

func stringsDiff(a, b []string) []string {
	r := []string{}

	for _, v := range a {
		if !slices.Contains(b, v) {
			r = append(r, v)
		}
	}

	return r
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
			params := &core.GetTrashObjectParams{}
			if trashObject.Id != nil {
				params.TrashObjectId = trashObject.Id
			} else {
				params.TrashObjectObjectId = trashObject.ObjectId
			}
			_, e := m.Core.GetTrashObjectWithResponse(ctx, params)
			if e != nil {
				if errors.Is(e, core.ErrNotFound) {
					return 1, "not_found", nil
				}

				return nil, "", e
			}

			return nil, "exists", nil
		},
		Timeout:                   timeout,
		Delay:                     m.stateChangeDelay(1 * time.Second),
		MinTimeout:                m.stateChangeDelay(5 * time.Second),
		PollInterval:              m.stateChangePollInterval(),
		ContinuousTargetOccurence: 1,
	}

	_, err := waiter.WaitForStateContext(ctx)

	return err
}
