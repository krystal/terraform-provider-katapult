package provider

import (
	"context"
	"errors"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/krystal/go-katapult"
	"github.com/krystal/go-katapult/core"
)

func stringSliceToSchemaSet(s []string) *schema.Set {
	set := &schema.Set{F: schema.HashString}
	for _, v := range s {
		set.Add(v)
	}

	return set
}

func schemaSetToSlice[T any](s *schema.Set) []T {
	if s == nil {
		return nil
	}

	r := make([]T, 0, s.Len())
	for _, v := range s.List() {
		r = append(r, v.(T))
	}

	return r
}

func purgeTrashObjectByObjectID(
	ctx context.Context,
	m *Meta,
	timeout time.Duration,
	objectID string,
) error {
	return purgeTrashObject(
		ctx, m, timeout, &core.TrashObject{ObjectID: objectID},
	)
}

func purgeTrashObject(
	ctx context.Context,
	m *Meta,
	timeout time.Duration,
	trash *core.TrashObject,
) error {
	ref := trash.Ref()
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
	waiter := &resource.StateChangeConf{
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

func waitForTaskCompletion(
	ctx context.Context,
	m *Meta,
	timeout time.Duration,
	task *core.Task,
) error {
	taskWaiter := &resource.StateChangeConf{
		Pending: []string{
			string(core.TaskPending),
			string(core.TaskRunning),
		},
		Target: []string{
			string(core.TaskCompleted),
		},
		Refresh: func() (interface{}, string, error) {
			t, _, e := m.Core.Tasks.Get(ctx, task.ID)
			if e != nil {
				return t, "", e
			}
			if t.Status == core.TaskFailed {
				return t, string(t.Status), errors.New("task failed")
			}

			return t, string(t.Status), nil
		},
		Timeout:                   timeout,
		Delay:                     1 * time.Second,
		MinTimeout:                5 * time.Second,
		ContinuousTargetOccurence: 1,
	}

	_, err := taskWaiter.WaitForStateContext(ctx)

	return err
}

func stringsDiff(a, b []string) []string {
	r := []string{}

	for _, v := range a {
		if !stringsContain(b, v) {
			r = append(r, v)
		}
	}

	return r
}

func stringsContain(strs []string, s string) bool {
	if len(strs) == 0 || s == "" {
		return false
	}

	for _, v := range strs {
		if v == s {
			return true
		}
	}

	return false
}

func stringsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	if len(stringsDiff(a, b)) > 0 {
		return false
	}

	if len(stringsDiff(b, a)) > 0 {
		return false
	}

	return true
}

func mapKeys[T comparable, V any](m map[T]V) []T {
	keys := make([]T, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	return keys
}
