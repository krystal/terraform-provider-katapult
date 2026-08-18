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

const virtualMachineDiskDefaultPageSize = 30

const (
	stateAttributeName = "state"
	unknownStateValue  = "unknown"
)

// fetchAllVMDisks returns every disk attachment for a given VM, paging as needed.
func fetchAllVMDisks(
	ctx context.Context,
	m *Meta,
	vmID string,
) ([]core.GetVirtualMachineDisks200ResponseDisks, error) {
	var all []core.GetVirtualMachineDisks200ResponseDisks
	for page := 1; ; page++ {
		p := page
		res, err := m.Core.GetVirtualMachineDisksWithResponse(ctx,
			&core.GetVirtualMachineDisksParams{
				VirtualMachineId: &vmID,
				Page:             &p,
			})
		if err != nil {
			if res != nil && isErrNotFoundOrInTrash(err, res.JSON406) {
				return nil, core.ErrNotFound
			}
			if res != nil {
				return nil, genericAPIError(err, res.Body)
			}
			return nil, err
		}
		if res.JSON200 == nil {
			return nil, fmt.Errorf("unexpected empty response fetching VM disks")
		}
		body := res.JSON200
		all = append(all, body.Disks...)
		if !paginationHasNext(
			body.Pagination, page, len(body.Disks), virtualMachineDiskDefaultPageSize,
		) {
			break
		}
	}
	return all, nil
}

func isErrNotFoundOrInTrash(err error, res *core.ObjectInTrashResponse) bool {
	return errors.Is(err, core.ErrNotFound) ||
		(res != nil && res.Code != nil &&
			*res.Code == core.ObjectInTrashEnumObjectInTrash)
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

	res, err := m.Core.DeleteTrashObjectWithResponse(ctx,
		core.DeleteTrashObjectJSONRequestBody{
			TrashObject: lookup,
		})
	if err != nil {
		if res != nil && res.JSON404 != nil {
			return nil
		}
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
			if res == nil || res.JSON200 == nil {
				return nil, "", fmt.Errorf(
					"unexpected empty response fetching task",
				)
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

func waitForDiskSize(
	ctx context.Context,
	m *Meta,
	diskID string,
	expected int64,
	timeout time.Duration,
) error {
	target := fmt.Sprintf("%d", expected)
	waiter := &retry.StateChangeConf{
		Pending:      []string{unknownStateValue},
		Target:       []string{target},
		Timeout:      timeout,
		Delay:        m.stateChangeDelay(time.Second),
		MinTimeout:   m.stateChangeDelay(2 * time.Second),
		PollInterval: m.stateChangePollInterval(),
		Refresh: func() (interface{}, string, error) {
			res, err := m.Core.GetDiskWithResponse(ctx, &core.GetDiskParams{DiskId: &diskID})
			if err != nil {
				if res != nil {
					err = genericAPIError(err, res.Body)
				}
				return nil, "", err
			}
			if res == nil || res.JSON200 == nil || res.JSON200.Disk.SizeInGb == nil {
				return nil, unknownStateValue, nil
			}
			size := int64(*res.JSON200.Disk.SizeInGb)
			state := fmt.Sprintf("%d", size)
			if size != expected {
				state = unknownStateValue
			}
			return &res.JSON200.Disk, state, nil
		},
	}
	_, err := waiter.WaitForStateContext(ctx)
	if err != nil {
		return fmt.Errorf("waiting for disk %s size %d GB: %w", diskID, expected, err)
	}
	return nil
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
			res, err := m.Core.GetTrashObjectWithResponse(ctx, params)
			if err != nil {
				if errors.Is(err, core.ErrNotFound) {
					return 1, "not_found", nil
				}
				if res != nil {
					err = genericAPIError(err, res.Body)
				}

				return nil, "", err
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
