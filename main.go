package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/chieftools/terraform-provider-domainchief/internal/provider"
)

// version is set by GoReleaser.
var version = "dev"

func main() {
	var debug bool

	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/chieftools/domainchief",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err.Error())
	}
}
