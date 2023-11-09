package v6provider

import (
	"context"
	"log"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestMain(m *testing.M) {
	resource.TestMain(m)
}

func sweepMeta() *Meta {
	k := &KatapultProvider{Version: testAccProviderVersion}
	resp := &provider.ConfigureResponse{}
	k.Configure(
		context.TODO(),
		provider.ConfigureRequest{Config: tfsdk.Config{}},
		resp,
	)
	if resp.Diagnostics.HasError() {
		log.Fatalf("failed to configure client: %+v", resp.Diagnostics)
	}

	return k.m
}
