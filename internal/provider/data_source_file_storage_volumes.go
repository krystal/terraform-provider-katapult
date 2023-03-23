package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/krystal/go-katapult/core"
)

func dataSourceFileStorageVolumes() *schema.Resource {
	fsv := dataSourceFileStorageVolume().Schema

	return &schema.Resource{
		ReadContext: dataSourceFileStorageVolumesRead,
		Description: "Fetch all file storage volumes in the organization.",
		Schema: map[string]*schema.Schema{
			"id": {
				Description: "Always set to provider organization value.",
				Type:        schema.TypeString,
				Computed:    true,
			},
			"file_storage_volumes": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: fsv,
				},
			},
		},
	}
}

func dataSourceFileStorageVolumesRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)
	var diags diag.Diagnostics

	volumes, err := getAllFlattenedFileStorageVolumes(ctx, m, m.OrganizationRef)
	if err != nil {
		return diag.FromErr(err)
	}

	err = d.Set("file_storage_volumes", volumes)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(m.confOrganization)

	return diags
}

func getAllFlattenedFileStorageVolumes(
	ctx context.Context,
	m *Meta,
	orgRef core.OrganizationRef,
) ([]map[string]any, error) {
	var volumes []map[string]any
	totalPages := 2

	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		pageResult, resp, err := m.Core.FileStorageVolumes.List(
			ctx, orgRef, &core.ListOptions{Page: pageNum},
		)
		if err != nil {
			return nil, err
		}

		totalPages = resp.Pagination.TotalPages

		for _, fsv := range pageResult {
			volumes = append(volumes, flattenFileStorageVolume(fsv))
		}
	}

	return volumes, nil
}

func flattenFileStorageVolume(fsv *core.FileStorageVolume) map[string]any {
	return map[string]any{
		"id":           fsv.ID,
		"name":         fsv.Name,
		"associations": stringSliceToSchemaSet(fsv.Associations),
		"size":         fsv.Size,
		"nfs_location": fsv.NFSLocation,
	}
}
