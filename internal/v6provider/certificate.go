package v6provider

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/core"
)

func ConvertCoreCertsToTFValues(certs []core.Certificate) []attr.Value {
	values := make([]attr.Value, len(certs))
	for i, cert := range certs {
		values[i] = types.StringValue(cert.ID)
	}
	return values
}
