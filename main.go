package main

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/plugin"
	"github.com/krystal/terraform-provider-katapult/internal/provider"
)

var (
	Version string = "dev"
	Commit  string = ""
	Date    string = ""
)

func main() {
	plugin.Serve(&plugin.ServeOpts{
		ProviderFunc: provider.New(&provider.Config{
			Version: Version,
			Commit:  Commit,
			Date:    Date,
		}),
	})
}
