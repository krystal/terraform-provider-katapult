package provider

import (
	"context"
	"errors"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/krystal/go-katapult/core"
	"github.com/krystal/terraform-provider-katapult/internal/hashcode"
)

func stringHash(v interface{}) int {
	return hashcode.String(v.(string))
}

func newSchemaStringSet(strs []string) *schema.Set {
	var v []interface{}
	for _, id := range strs {
		v = append(v, id)
	}

	return schema.NewSet(stringHash, v)
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
	task, resp, err := m.Core.TrashObjects.Purge(ctx, trash.Ref())
	if err != nil {
		if resp != nil && resp.Response != nil && resp.StatusCode == 404 {
			return nil
		}

		return err
	}

	_, err = waitForTaskCompletion(ctx, m, timeout, task)

	return err
}

func waitForTaskCompletion(
	ctx context.Context,
	m *Meta,
	timeout time.Duration,
	task *core.Task,
) (*core.Task, error) {
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
				return 0, "", e
			}
			if t.Status == core.TaskFailed {
				return 0, string(t.Status), errors.New("task failed")
			}

			return t, string(t.Status), nil
		},
		Timeout:                   timeout,
		Delay:                     1 * time.Second,
		MinTimeout:                5 * time.Second,
		ContinuousTargetOccurence: 1,
	}

	t, err := taskWaiter.WaitForStateContext(ctx)
	if tsk, ok := t.(*core.Task); ok {
		return tsk, err
	}

	return nil, err
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
