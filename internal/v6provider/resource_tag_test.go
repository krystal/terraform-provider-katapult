package v6provider

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/next/core"
)

func init() { //nolint:gochecknoinits
	resource.AddTestSweepers("katapult_resource_tag", &resource.Sweeper{
		Name: "katapult_resource_tag",
		F:    testSweepTags,
	})
}

func testSweepTags(_ string) error {
	var pageSize int = 200

	m := sweepMeta()
	ctx := context.TODO()

	var tags []core.GetOrganizationTags200ResponseTags
	totalPages := 2
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		res, err := m.Core.GetOrganizationTagsWithResponse(ctx,
			&core.GetOrganizationTagsParams{
				OrganizationId: &m.confOrganization,
				Page:           &pageNum,
				PerPage:        &pageSize,
			})
		if err != nil {
			return err
		}

		resp := res.JSON200

		totalPages, _ = resp.Pagination.TotalPages.Get()
		tags = append(tags, resp.Tags...)
	}

	for _, tag := range tags {
		if !strings.HasPrefix(*tag.Name, testAccResourceNamePrefix) {
			continue
		}

		m.Logger.Info("deleting tag", "id", *tag.Id, "name", *tag.Name)

		_, err := m.Core.DeleteTagWithResponse(ctx,
			core.DeleteTagJSONRequestBody{
				Tag: core.TagLookup{
					Id: tag.Id,
				},
			})
		if err != nil {
			return err
		}
	}

	return nil
}

func TestAccKatapultTag_minimal(t *testing.T) {
	tt := newTestTools(t)
	name := strings.ToLower(tt.ResourceName())

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultTagDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
				resource "katapult_tag" "db" {
					name = "%s"
				}`,
					name),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultTagAttrs(
						tt, "katapult_tag.db",
					),
				),
			},
			{
				ResourceName:      "katapult_tag.db",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccKatapultTag_update_color(t *testing.T) {
	tt := newTestTools(t)
	name := strings.ToLower(tt.ResourceName())

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultTagDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
				resource "katapult_tag" "kv" {
					name = "%s"
					color = "red"
				}`,
					name),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultTagAttrs(
						tt, "katapult_tag.kv",
					),
				),
			},

			{
				Config: undent.Stringf(`
			resource "katapult_tag" "kv" {
				name = "%s"
				color = "yellow"

			}`,
					name),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultTagAttrs(
						tt, "katapult_tag.kv",
					),
				),
			},
		},
	})
}

//
// Helpers
//

func testAccCheckKatapultTagAttrs(
	tt *testTools,
	res string,
) resource.TestCheckFunc {
	m := tt.Meta

	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[res]
		if !ok {
			return fmt.Errorf("resource not found: %s", res)
		}

		resp, err := m.Core.GetTagWithResponse(tt.Ctx,
			&core.GetTagParams{
				TagId: &rs.Primary.ID,
			})
		if err != nil {
			return err
		}

		tag := resp.JSON200.Tag

		tfs := []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(res, "id", *tag.Id),
			resource.TestCheckResourceAttr(res, "name", *tag.Name),
			resource.TestCheckResourceAttr(
				res, "color", string(*tag.Color),
			),
		}

		return resource.ComposeAggregateTestCheckFunc(tfs...)(s)
	}
}

func testAccCheckKatapultTagDestroy(
	tt *testTools,
) resource.TestCheckFunc {
	m := tt.Meta

	return func(s *terraform.State) error {
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "katapult_load_tag" {
				continue
			}

			resp, err := m.Core.GetTagWithResponse(tt.Ctx,
				&core.GetTagParams{
					TagId: &rs.Primary.ID,
				})

			if err == nil && resp.JSON200 != nil {
				return fmt.Errorf(
					"katapult_tag %s (%s) was not destroyed",
					rs.Primary.ID, string(*resp.JSON200.Tag.Name),
				)
			}
		}

		return nil
	}
}
