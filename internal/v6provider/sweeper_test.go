package v6provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestMain(m *testing.M) {
	resource.TestMain(m)
}

func sweepMeta() *Meta {
	meta, err := NewMeta("", "", "", nil, "",
		testAccResourceNamePrefix, nil, "", "")
	if err != nil {
		return nil
	}

	return meta
}
