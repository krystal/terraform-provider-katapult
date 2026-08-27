package v6provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

func runDataSourceV5Handover(
	t *testing.T,
	config string,
	legacyCheck func(*testTools) resource.TestCheckFunc,
	frameworkCheck func(*testTools) resource.TestCheckFunc,
) {
	t.Helper()
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{
				ProtoV5ProviderFactories: tt.LegacyDataSourceFactories,
				Config:                   config,
				Check:                    legacyCheck(tt),
			},
			{
				ProtoV6ProviderFactories: tt.ProviderFactories,
				Config:                   config,
				Check:                    frameworkCheck(tt),
			},
			{
				ProtoV6ProviderFactories: tt.ProviderFactories,
				Config:                   config,
				PlanOnly:                 true,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}
