package v6provider

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/core"
)

type (
	CertificateModel struct {
		ID              types.String `tfsdk:"id"`
		Name            types.String `tfsdk:"name"`
		AdditionalNames types.List   `tfsdk:"additional_names"`
		State           types.String `tfsdk:"state"`
	}
)

func CertificateResourceSchemaAtrributes() map[string]rschema.Attribute {
	return map[string]rschema.Attribute{
		"id": rschema.StringAttribute{
			Required: true,
		},
		"name": rschema.StringAttribute{
			Required: true,
		},
		"additional_names": rschema.ListAttribute{
			Optional:    true,
			Computed:    true,
			ElementType: types.StringType,
		},

		"state": rschema.StringAttribute{
			Computed: true,
		},
	}
}

func CertificateDataSourceSchemaAtrributes() map[string]dschema.Attribute {
	return map[string]dschema.Attribute{
		"id": dschema.StringAttribute{
			Required: true,
		},
		"name": dschema.StringAttribute{
			Required: true,
		},
		"additional_names": dschema.ListAttribute{
			Computed:    true,
			ElementType: types.StringType,
		},

		"state": dschema.StringAttribute{
			Computed: true,
		},
	}
}

func CertificateType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"id":   types.StringType,
			"name": types.StringType,
			"additional_names": types.ListType{
				ElemType: types.StringType,
			},
			"state": types.StringType,
		},
	}
}

func ConvertCoreCertsToTFValues(certs []core.Certificate) []attr.Value {
	values := make([]attr.Value, len(certs))
	for i, cert := range certs {
		values[i] = types.StringValue(cert.ID)
	}
	return values
}
