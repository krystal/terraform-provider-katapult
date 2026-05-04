package v6provider

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/krystal/go-katapult/next/core"
)

func objectStorageAccountGet(
	ctx context.Context,
	m *Meta,
	region string,
) (core.ObjectStorageAccountProvisioningStateEnum, error) {
	res, err := m.Core.
		GetOrganizationObjectStorageObjectStorageClusterWithResponse(ctx,
			&core.GetOrganizationObjectStorageObjectStorageClusterParams{
				OrganizationSubDomain:      &m.confOrganization,
				ObjectStorageClusterRegion: &region,
			})
	if err != nil {
		body := ""
		if res != nil {
			body = string(res.Body)
		}

		return core.ObjectStorageAccountProvisioningStateEnumFailed,
			fmt.Errorf("%w: %s", err, body)
	}

	return *res.JSON200.ObjectStorageAccount.ProvisioningState, nil
}

func objectStorageAccountCreate(
	ctx context.Context,
	m *Meta,
	region string,
) error {
	res, err := m.Core.
		PostOrganizationObjectStorageObjectStorageClusterWithResponse(
			ctx,

			core.PostOrganizationObjectStorageObjectStorageClusterJSONRequestBody{
				ObjectStorageCluster: core.ObjectStorageClusterLookup{
					Region: &region,
				},
				Organization: core.OrganizationLookup{
					SubDomain: &m.confOrganization,
				},
			})
	if err != nil {
		body := ""
		if res != nil {
			body = string(res.Body)
		}

		return fmt.Errorf("%w: %s", err, body)
	}

	return nil
}

func ensureObjectStorageAccount(
	ctx context.Context,
	m *Meta,
	region string,
) error {
	state, err := objectStorageAccountGet(ctx, m, region)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			if createErr := objectStorageAccountCreate(
				ctx, m, region,
			); createErr != nil {
				return createErr
			}
		} else {
			return err
		}
	}

	if state == core.ObjectStorageAccountProvisioningStateEnumProvisioned {
		return nil
	}

	waiter := &retry.StateChangeConf{
		Pending: []string{
			string(
				core.ObjectStorageAccountProvisioningStateEnumFailed,
			),
			string(
				core.ObjectStorageAccountProvisioningStateEnumProvisioning,
			),
		},
		Target: []string{
			string(
				core.ObjectStorageAccountProvisioningStateEnumProvisioned,
			),
		},
		Refresh: func() (interface{}, string, error) {
			s, stateErr := objectStorageAccountGet(ctx, m, region)
			if stateErr != nil {
				return 0, "", stateErr
			}

			return 1, string(s), nil
		},
		Timeout:                   5 * time.Minute,
		Delay:                     2 * time.Second,
		MinTimeout:                5 * time.Second,
		ContinuousTargetOccurence: 1,
	}

	_, err = waiter.WaitForStateContext(ctx)

	return err
}
