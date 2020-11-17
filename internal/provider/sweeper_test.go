package provider

import (
	"context"
	"log"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestMain(m *testing.M) {
	resource.TestMain(m)
}

func sweepMeta() *Meta {
	conf := &Config{Version: testAccProviderVersion}
	p := New(conf)()

	d := p.Configure(context.TODO(), terraform.NewResourceConfigRaw(nil))
	if d.HasError() {
		log.Fatalf("failed to configure client: %+v", d)
	}

	return p.Meta().(*Meta)
}
