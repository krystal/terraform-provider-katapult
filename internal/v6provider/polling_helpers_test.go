package v6provider

import (
	"context"
	"testing"
	"time"

	"github.com/krystal/go-katapult/next/core"
	"github.com/stretchr/testify/require"
)

type testFSV = core.GetFileStorageVolume200ResponseFileStorageVolume

type pollingCoreClient struct {
	core.ClientWithResponsesInterface

	getFileStorageVolume func(
		context.Context,
		*core.GetFileStorageVolumeParams,
		...core.RequestEditorFn,
	) (*core.GetFileStorageVolumeResponse, error)
	getTrashObject func(
		context.Context,
		*core.GetTrashObjectParams,
		...core.RequestEditorFn,
	) (*core.GetTrashObjectResponse, error)
}

func (c *pollingCoreClient) GetFileStorageVolumeWithResponse(
	ctx context.Context,
	params *core.GetFileStorageVolumeParams,
	reqEditors ...core.RequestEditorFn,
) (*core.GetFileStorageVolumeResponse, error) {
	return c.getFileStorageVolume(ctx, params, reqEditors...)
}

func (c *pollingCoreClient) GetTrashObjectWithResponse(
	ctx context.Context,
	params *core.GetTrashObjectParams,
	reqEditors ...core.RequestEditorFn,
) (*core.GetTrashObjectResponse, error) {
	return c.getTrashObject(ctx, params, reqEditors...)
}

func TestWaitForTrashObjectNotFoundReturnsAPIError(t *testing.T) {
	calls := 0
	client := &pollingCoreClient{
		getTrashObject: func(
			context.Context,
			*core.GetTrashObjectParams,
			...core.RequestEditorFn,
		) (*core.GetTrashObjectResponse, error) {
			calls++

			return &core.GetTrashObjectResponse{
				Body: []byte(`{
					"error": {
						"code": "permission_denied",
						"description": "Not permitted"
					}
				}`),
			}, core.ErrRequestFailed
		},
	}
	meta := &Meta{Core: client, testMode: true}

	err := waitForTrashObjectNotFound(
		context.Background(), meta, time.Second, core.TrashObject{},
	)

	require.EqualError(t, err, "permission_denied: Not permitted")
	require.Equal(t, 1, calls)
}

func TestWaitForTrashObjectNotFoundHandlesNotFound(t *testing.T) {
	client := &pollingCoreClient{
		getTrashObject: func(
			context.Context,
			*core.GetTrashObjectParams,
			...core.RequestEditorFn,
		) (*core.GetTrashObjectResponse, error) {
			return &core.GetTrashObjectResponse{}, core.ErrNotFound
		},
	}
	meta := &Meta{Core: client, testMode: true}

	err := waitForTrashObjectNotFound(
		context.Background(), meta, time.Second, core.TrashObject{},
	)

	require.NoError(t, err)
}

func TestWaitForFileStorageVolumeToBeReadyReturnsAPIError(t *testing.T) {
	client := &pollingCoreClient{
		getFileStorageVolume: func(
			context.Context,
			*core.GetFileStorageVolumeParams,
			...core.RequestEditorFn,
		) (*core.GetFileStorageVolumeResponse, error) {
			return &core.GetFileStorageVolumeResponse{
				Body: []byte(`{
					"error": {
						"code": "permission_denied",
						"description": "Not permitted"
					}
				}`),
			}, core.ErrRequestFailed
		},
	}
	meta := &Meta{Core: client, testMode: true}

	volume, err := waitForFileStorageVolumeToBeReady(
		context.Background(), meta, time.Second, 0, nil,
	)

	require.Nil(t, volume)
	require.EqualError(t, err, "permission_denied: Not permitted")
}

func TestWaitForFileStorageVolumeToBeReadyRejectsEmptyResponse(t *testing.T) {
	client := &pollingCoreClient{
		getFileStorageVolume: func(
			context.Context,
			*core.GetFileStorageVolumeParams,
			...core.RequestEditorFn,
		) (*core.GetFileStorageVolumeResponse, error) {
			return &core.GetFileStorageVolumeResponse{}, nil
		},
	}
	meta := &Meta{Core: client, testMode: true}

	volume, err := waitForFileStorageVolumeToBeReady(
		context.Background(), meta, time.Second, 0, nil,
	)

	require.Nil(t, volume)
	require.EqualError(t, err, "unexpected empty file storage volume response")
}

func TestWaitForFileStorageVolumeToBeReadyRejectsMissingState(t *testing.T) {
	client := &pollingCoreClient{
		getFileStorageVolume: func(
			context.Context,
			*core.GetFileStorageVolumeParams,
			...core.RequestEditorFn,
		) (*core.GetFileStorageVolumeResponse, error) {
			return fileStorageVolumeResponse(nil), nil
		},
	}
	meta := &Meta{Core: client, testMode: true}

	volume, err := waitForFileStorageVolumeToBeReady(
		context.Background(), meta, time.Second, 0, nil,
	)

	require.Nil(t, volume)
	require.EqualError(
		t, err, "unexpected file storage volume response: missing state",
	)
}

func TestWaitForFileStorageVolumeToBeReadyReturnsReadyVolume(t *testing.T) {
	state := core.FileStorageVolumeStateEnumReady
	client := &pollingCoreClient{
		getFileStorageVolume: func(
			context.Context,
			*core.GetFileStorageVolumeParams,
			...core.RequestEditorFn,
		) (*core.GetFileStorageVolumeResponse, error) {
			return fileStorageVolumeResponse(&state), nil
		},
	}
	meta := &Meta{Core: client, testMode: true}

	volume, err := waitForFileStorageVolumeToBeReady(
		context.Background(), meta, time.Second, 0, nil,
	)

	require.NoError(t, err)
	require.NotNil(t, volume)
	require.Equal(t, state, *volume.State)
}

func fileStorageVolumeResponse(
	state *core.FileStorageVolumeStateEnum,
) *core.GetFileStorageVolumeResponse {
	return &core.GetFileStorageVolumeResponse{
		JSON200: &struct {
			Annotations       []core.KeyValue `json:"annotations"`
			FileStorageVolume testFSV         `json:"file_storage_volume"`
		}{
			FileStorageVolume: testFSV{
				State: state,
			},
		},
	}
}
