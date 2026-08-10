package main

import (
	"context"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6/tf6server"
	"github.com/hashicorp/terraform-plugin-mux/tf5to6server"
	"github.com/hashicorp/terraform-plugin-mux/tf6muxserver"
	"github.com/krystal/terraform-provider-katapult/internal/provider"
	"github.com/krystal/terraform-provider-katapult/internal/v6provider"
)

var (
	version = "dev"
	commit  = ""
)

func main() {
	ctx := context.Background()
	providerServer, err := newProviderServer(ctx)
	if err != nil {
		log.Fatal(err)
	}

	err = tf6server.Serve(
		"registry.terraform.io/providers/krystal/katapult",
		providerServer,
	)
	if err != nil {
		log.Fatal(err)
	}
}

func newProviderServer(
	ctx context.Context,
) (func() tfprotov6.ProviderServer, error) {
	upgradedSDKServer, err := tf5to6server.UpgradeServer(
		ctx, provider.New(&provider.Config{
			Version: version,
			Commit:  commit,
		})().GRPCProvider,
	)
	if err != nil {
		return nil, err
	}

	providers := []func() tfprotov6.ProviderServer{
		func() tfprotov6.ProviderServer { return upgradedSDKServer },
		providerserver.NewProtocol6(v6provider.New(&v6provider.KatapultProvider{
			Version: version,
			Commit:  commit,
		})()),
	}

	muxServer, err := tf6muxserver.NewMuxServer(ctx, providers...)
	if err != nil {
		return nil, err
	}

	return muxServer.ProviderServer, nil
}
