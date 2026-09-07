package v6provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/krystal/go-katapult/next/core"
)

// resourceDeletionCheck returns true only when the original resource is absent.
// A resource still in trash is pending; a live resource or failed lookup is an error.
type resourceDeletionCheck func(context.Context) (bool, error)

func resourceDeleted(
	name string, err error, inTrash *core.ObjectInTrashResponse, body []byte,
) (bool, error) {
	if errors.Is(err, core.ErrNotFound) {
		return true, nil
	}
	if err != nil {
		if isErrNotFoundOrInTrash(err, inTrash) {
			return false, nil
		}
		return false, fmt.Errorf("failed to verify deletion of %s: %w", name, genericAPIError(err, body))
	}
	return false, fmt.Errorf("%s still exists outside trash; it may have been restored or never moved to trash; "+
		"refusing to declare deletion complete", name)
}

func virtualMachineDeletionCheck(m *Meta, id string) resourceDeletionCheck {
	return func(ctx context.Context) (bool, error) {
		res, err := m.Core.GetVirtualMachineWithResponse(ctx, &core.GetVirtualMachineParams{VirtualMachineId: &id})
		if res == nil {
			return resourceDeleted("virtual machine "+id, err, nil, nil)
		}
		return resourceDeleted("virtual machine "+id, err, res.JSON406, res.Body)
	}
}

func diskDeletionCheck(m *Meta, id string) resourceDeletionCheck {
	return func(ctx context.Context) (bool, error) {
		res, err := m.Core.GetDiskWithResponse(ctx, &core.GetDiskParams{DiskId: &id})
		if res == nil {
			return resourceDeleted("disk "+id, err, nil, nil)
		}
		return resourceDeleted("disk "+id, err, res.JSON406, res.Body)
	}
}

func fileStorageVolumeDeletionCheck(m *Meta, id string) resourceDeletionCheck {
	return func(ctx context.Context) (bool, error) {
		res, err := m.Core.GetFileStorageVolumeWithResponse(ctx,
			&core.GetFileStorageVolumeParams{FileStorageVolumeId: &id})
		if res == nil {
			return resourceDeleted("file storage volume "+id, err, nil, nil)
		}
		return resourceDeleted("file storage volume "+id, err, res.JSON406, res.Body)
	}
}

func objectStorageAccountDeletionCheck(m *Meta, region string) resourceDeletionCheck {
	return func(ctx context.Context) (bool, error) {
		res, err := m.Core.GetOrganizationObjectStorageObjectStorageClusterWithResponse(ctx,
			&core.GetOrganizationObjectStorageObjectStorageClusterParams{
				OrganizationSubDomain: &m.confOrganization, ObjectStorageClusterRegion: &region,
			})
		if res == nil {
			return resourceDeleted("object storage account in "+region, err, nil, nil)
		}
		return resourceDeleted("object storage account in "+region, err, res.JSON406, res.Body)
	}
}
