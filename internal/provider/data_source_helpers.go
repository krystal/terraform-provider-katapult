package provider

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

// dataSourceSchemaFromResourceSchema converts a resource schema to one that is
// suitable for data sources.
func dataSourceSchemaFromResourceSchema(
	s map[string]*schema.Schema,
) map[string]*schema.Schema {
	ds := make(map[string]*schema.Schema, len(s))

	for k, v := range s {
		dv := &schema.Schema{
			Type:        v.Type,
			Description: v.Description,
			Computed:    true,
			ForceNew:    false,
		}

		switch v.Type {
		case schema.TypeSet:
			dv.Set = v.Set
		case schema.TypeList:
			if elem, ok := v.Elem.(*schema.Resource); ok {
				dv.Elem = &schema.Resource{
					Schema: dataSourceSchemaFromResourceSchema(elem.Schema),
				}
			} else {
				dv.Elem = v.Elem
			}
		default:
			dv.Elem = v.Elem
		}

		ds[k] = dv
	}

	return ds
}
