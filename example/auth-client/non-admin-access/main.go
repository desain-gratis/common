package main

import (
	"context"
	"net/http"
	"os"

	authclient "github.com/desain-gratis/common/delivery/auth-api-client"
	mycontentapiclient "github.com/desain-gratis/common/delivery/mycontent-api-client"
	"github.com/desain-gratis/common/example/auth/entity"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func init() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Logger()
}

func main() {
	// gcloud auth list
	// Add auth:
	// 1. Download service-credential first to KEY_FILE
	//     gcloud auth activate-service-account [ACCOUNT] --key-file=KEY_FILE
	// 2. delete KEY_FILE
	// export GOOGLE_ID_TOKEN=$(gcloud auth print-identity-token --audiences='807095026235-ghkps0coeukr7ckr5398vcc66tpnqpuj.apps.googleusercontent.com' --account='desain-gratis-developer@langsunglelang.iam.gserviceaccount.com')
	idToken := os.Getenv("GOOGLE_ID_TOKEN")
	if idToken == "" {
		log.Fatal().Msgf("Please specify GOOGLE_ID_TOKEN environment variables with google id token")
	}

	// user token
	aca := authclient.New("http://localhost:9090/auth/signin")
	ar, errUC := aca.SignIn(context.Background(), idToken)
	if errUC != nil {
		log.Fatal().Msgf("Err get google ID %v", errUC)
	}

	log.Info().Msgf("ar %+v", *ar.IDToken)

	projectClient := mycontentapiclient.New[*entity.Project](http.DefaultClient, "http://localhost:9090/project", []string{})
	_, err := projectClient.Get(context.Background(), *ar.IDToken, "*", nil, "")
	if err != nil {
		log.Info().Msgf("THIS ERROR IS EXPECTED, NOT AN ADMIN CANNOT USE '*' namespace: %+v", err)
	}

	authorizedUserClient := mycontentapiclient.New[*entity.UserAuthorization](
		http.DefaultClient,
		"http://localhost:9090/auth/user",
		nil,
	)

	_, err = authorizedUserClient.Get(context.Background(), *ar.IDToken, "new-project", nil, "")
	if err != nil {
		log.Info().Msgf("THIS ERROR IS EXPECTED, NOT AN ADMIN: %+v", err)
	}

	_, err = authorizedUserClient.Get(context.Background(), *ar.IDToken, "*", nil, "")
	if err != nil {
		log.Info().Msgf("THIS ERROR IS EXPECTED, NOT AN ADMIN: %+v", err)
	}

	proj, err := projectClient.Get(context.Background(), *ar.IDToken, "new-project", nil, "")
	if err != nil {
		log.Error().Msgf("THIS ERROR IS NOT EXPECTED!!! %+v", err)
	}

	log.Info().Msgf("RESULT: %+v", proj[0])
}
