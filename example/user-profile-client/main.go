package main

import (
	"context"
	"flag"
	"net/url"
	"os"

	contentsync "github.com/desain-gratis/common/delivery/mycontent-api-client"
	"github.com/desain-gratis/common/example/user-profile/entity"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func init() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Logger()
}

func main() {
	var reset bool
	flag.BoolVar(&reset, "reset", false, "reset all data in the app")
	flag.Parse()

	orgEndpoint, _ := url.Parse("http://localhost:9090/org")
	userEndpoint, _ := url.Parse("http://localhost:9090/org/user")

	if reset {
		sampleUser = nil
	}

	orgSync := contentsync.Builder[*entity.Organization](orgEndpoint).
		WithNamespace("*").
		WithData(sampleOrg)

	userSync := contentsync.Builder[*entity.UserProfile](userEndpoint, "org_id").
		WithNamespace("*").
		WithData(sampleUser)

	userSync.
		WithImages(getUserProfileImage, "./thumbnail", "profile_id").
		WithUploadDirectory(".")

	ctx := context.Background()

	orgSync.Build().Execute(ctx)
	userSync.Build().Execute(ctx)
}
