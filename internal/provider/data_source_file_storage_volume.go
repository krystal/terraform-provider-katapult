package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceFileStorageVolume() *schema.Resource {
	ds := dataSourceSchemaFromResourceSchema(resourceFileStorageVolume().Schema)

	ds["id"] = &schema.Schema{
		Type:        schema.TypeString,
		Required:    true,
		Description: "The ID of this resource.",
	}
	ds["size"] = &schema.Schema{
		Type:        schema.TypeInt,
		Computed:    true,
		Description: "The size of the volume in bytes.",
	}

	return &schema.Resource{
		ReadContext: dataSourceFileStorageVolumeRead,
		Schema:      ds,
		Description: "Fetch details for a individual file storage volume.",
	}
}

func dataSourceFileStorageVolumeRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)
	var diags diag.Diagnostics

	fsv, _, err := m.Core.FileStorageVolumes.GetByID(ctx, d.Get("id").(string))
	if err != nil {
		return diag.FromErr(err)
	}

	_ = d.Set("name", fsv.Name)
	_ = d.Set("size", fsv.Size)
	_ = d.Set("nfs_location", fsv.NFSLocation)

	err = d.Set("associations", stringSliceToSchemaSet(fsv.Associations))
	if err != nil {
		diags = append(diags, diag.FromErr(err)...)
	}

	d.SetId(fsv.ID)

	return diags
}
