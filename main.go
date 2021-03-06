package main

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/plugin"
	"github.com/krystal/terraform-provider-katapult/internal/provider"
)

var (
	version = "dev"
	commit  = ""
)

func main() {
	plugin.Serve(&plugin.ServeOpts{
		ProviderFunc: provider.New(&provider.Config{
			Version: version,
			Commit:  commit,
		}),
	})
}
