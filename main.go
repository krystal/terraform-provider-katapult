package main

import (
	"context"
	"log"

	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6/tf6server"
	"github.com/hashicorp/terraform-plugin-mux/tf5to6server"
	"github.com/hashicorp/terraform-plugin-mux/tf6muxserver"
	"github.com/krystal/terraform-provider-katapult/internal/provider"
)

var (
	version = "dev"
	commit  = ""
)

func main() {
	ctx := context.Background()

	upgradedSDKServer, err := tf5to6server.UpgradeServer(
		ctx, provider.New(&provider.Config{
			Version: version,
			Commit:  commit,
		})().GRPCProvider,
	)
	if err != nil {
		log.Fatal(err)
	}

	providers := []func() tfprotov6.ProviderServer{
		func() tfprotov6.ProviderServer { return upgradedSDKServer },
	}

	muxServer, err := tf6muxserver.NewMuxServer(ctx, providers...)
	if err != nil {
		log.Fatal(err)
	}

	err = tf6server.Serve(
		"registry.terraform.io/providers/krystal/katapult",
		muxServer.ProviderServer,
	)
	if err != nil {
		log.Fatal(err)
	}
}
