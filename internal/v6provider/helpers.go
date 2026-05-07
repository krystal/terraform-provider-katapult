package v6provider

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/krystal/go-katapult/next/core"
)

// requiresReplaceIfDecreased triggers replace when size_in_gb decreases
// because disks cannot be shrunk — only grown or replaced.
type requiresReplaceIfDecreasedModifier struct{}

func RequiresReplaceIfDecreased() planmodifier.Int64 {
	return requiresReplaceIfDecreasedModifier{}
}

func (m requiresReplaceIfDecreasedModifier) Description(
	_ context.Context,
) string {
	return "Requires replace if the value decreases."
}

func (m requiresReplaceIfDecreasedModifier) MarkdownDescription(
	_ context.Context,
) string {
	return "Requires replace if the value decreases."
}

func (m requiresReplaceIfDecreasedModifier) PlanModifyInt64(
	_ context.Context,
	req planmodifier.Int64Request,
	resp *planmodifier.Int64Response,
) {
	if req.PlanValue.IsUnknown() || req.StateValue.IsNull() {
		return
	}
	if req.PlanValue.ValueInt64() < req.StateValue.ValueInt64() {
		resp.RequiresReplace = true
	}
}

// fetchAllVMDisks returns every disk attachment for a given VM, paging as needed.
func fetchAllVMDisks(
	ctx context.Context,
	m *Meta,
	vmID string,
) ([]core.GetVirtualMachineDisks200ResponseDisks, error) {
	var all []core.GetVirtualMachineDisks200ResponseDisks
	totalPages := 1
	for page := 1; page <= totalPages; page++ {
		p := page
		res, err := m.Core.GetVirtualMachineDisksWithResponse(ctx,
			&core.GetVirtualMachineDisksParams{
				VirtualMachineId: &vmID,
				Page:             &p,
			})
		if err != nil {
			if res != nil {
				return nil, genericAPIError(err, res.Body)
			}
			return nil, err
		}
		if res.JSON200 == nil {
			return nil, fmt.Errorf("unexpected empty response fetching VM disks")
		}
		body := res.JSON200
		if body.Pagination.TotalPages.IsSpecified() {
			n, _ := body.Pagination.TotalPages.Get()
			totalPages = n
		}
		all = append(all, body.Disks...)
	}
	return all, nil
}

// isAdditionalDiskAttachment reports whether an attachment should be treated as
// a user-managed additional disk. The VM disks endpoint has historically
// omitted the boot flag for the system disk on some responses, so a nil boot
// flag is treated conservatively as boot and excluded from disk_ids state.
// Without that guard, the system disk can be misclassified as a user-managed
// additional disk.
func isAdditionalDiskAttachment(
	attachment core.GetVirtualMachineDisks200ResponseDisks,
) bool {
	return attachment.Boot != nil && !*attachment.Boot
}

// assignAndAttachDisksToVM assigns then attaches each disk in diskIDs to vmID.
func assignAndAttachDisksToVM(
	ctx context.Context,
	m *Meta,
	vmID string,
	diskIDs []string,
	timeout time.Duration,
) error {
	for _, id := range diskIDs {
		diskID := id
		assignRes, err := m.Core.PostDiskAssignWithResponse(ctx,
			core.PostDiskAssignJSONRequestBody{
				Disk:           core.DiskLookup{Id: &diskID},
				VirtualMachine: core.VirtualMachineLookup{Id: &vmID},
			})
		if err != nil {
			if assignRes != nil {
				return genericAPIError(err, assignRes.Body)
			}
			return err
		}

		attachRes, err := m.Core.PostDiskAttachWithResponse(ctx,
			core.PostDiskAttachJSONRequestBody{
				Disk: core.DiskLookup{Id: &diskID},
			})
		if err != nil {
			if attachRes != nil {
				return genericAPIError(err, attachRes.Body)
			}
			return err
		}
		if attachRes.JSON200 == nil || attachRes.JSON200.Task.Id == nil {
			return fmt.Errorf("unexpected empty response attaching disk %s", diskID)
		}
		if err := waitForTaskCompletion(
			ctx, m, timeout, *attachRes.JSON200.Task.Id,
		); err != nil {
			return err
		}
	}
	return nil
}

// detachAndUnassignDisk detaches then unassigns a disk from its VM.
// A 422 from detach (already detached) is silently ignored.
func detachAndUnassignDisk(
	ctx context.Context,
	m *Meta,
	diskID string,
	timeout time.Duration,
) error {
	detachRes, err := m.Core.PostDiskDetachWithResponse(ctx,
		core.PostDiskDetachJSONRequestBody{
			Disk: core.DiskLookup{Id: &diskID},
		})
	switch {
	case err == nil:
		if detachRes.JSON200 != nil && detachRes.JSON200.Task.Id != nil {
			if e := waitForTaskCompletion(
				ctx, m, timeout, *detachRes.JSON200.Task.Id,
			); e != nil {
				return e
			}
		}
	case detachRes != nil && detachRes.JSON422 != nil:
		// disk already detached — skip gracefully
	case detachRes != nil:
		return genericAPIError(err, detachRes.Body)
	default:
		return err
	}

	unassignRes, err := m.Core.PostDiskUnassignWithResponse(ctx,
		core.PostDiskUnassignJSONRequestBody{
			Disk: core.DiskLookup{Id: &diskID},
		})
	if err != nil {
		if unassignRes != nil {
			return genericAPIError(err, unassignRes.Body)
		}
		return err
	}

	return nil
}

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

	res, err := m.Core.DeleteTrashObjectWithResponse(ctx,
		core.DeleteTrashObjectJSONRequestBody{
			TrashObject: lookup,
		})
	if err != nil {
		if res.JSON404 != nil {
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
