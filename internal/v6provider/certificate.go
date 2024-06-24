package v6provider

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
)

func ConvertCoreCertsToTFValues(
	certs []core.GetLoadBalancersRulesLoadBalancerRulePartCertificates,
) []attr.Value {
	values := make([]attr.Value, len(certs))
	for i, cert := range certs {
		values[i] = types.StringPointerValue(cert.Id)
	}
	return values
}
