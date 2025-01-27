package main

import (
	"context"
	"net/http"
	"os"

	authclient "github.com/desain-gratis/common/delivery/auth-api-client"
	mycontentapiclient "github.com/desain-gratis/common/delivery/mycontent-api-client"
	"github.com/desain-gratis/common/example/auth/entity"
	signing_handler "github.com/desain-gratis/common/usecase/signing/handler"
	jwtrsa "github.com/desain-gratis/common/utility/secret/rsa"
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
	// export TOKEN=$(gcloud auth print-identity-token --audiences='807095026235-ghkps0coeukr7ckr5398vcc66tpnqpuj.apps.googleusercontent.com')
	idToken := os.Getenv("TOKEN")
	if idToken == "" {
		log.Fatal().Msgf("Please specify TOKEN environment variables with google id token")
	}

	// admin token
	aca := authclient.New("http://localhost:9090/auth/admin")
	ar, errUC := aca.SignIn(context.Background(), idToken)
	if errUC != nil {
		log.Fatal().Msgf("Err get google ID %v", errUC)
	}

	log.Info().Msgf("ar %+v", *ar.IDToken)

	kei := "desain-gratis-v1"

	// Expected failed because no key.
	_, err := jwtrsa.Default.ParseRSAJWTToken(*ar.IDToken, kei)
	if err != nil {
		log.Error().Msgf("THIS ERROR IS EXPCETED: %+v", err)
	}

	// store valid, but wrong key
	err = jwtrsa.Default.StorePublicKey(kei, `-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEZDXtDjpdz/oSbMh6rvspRNeu+WvI
8uoR41v79xmk6my0pgBsm06rm+u9uBdl2OlmNXoor+b6S7hDYbgtv55JYw==
-----END PUBLIC KEY-----`)
	if err != nil {
		log.Error().Msgf("THIS ERROR IS EXPCETED: %+v", err)
	}

	// Expected failed because of wrong key
	_, err = jwtrsa.Default.ParseRSAJWTToken(*ar.IDToken, kei)
	if err != nil {
		log.Error().Msgf("THIS ERROR IS EXPECTED: %+v", err)
	}

	// store valid key
	verifier := signing_handler.NewRemoteLoginVerifier(jwtrsa.Default, "http://localhost:9090/auth/keys")

	// direct from jwt library "non-proper" validation
	a, err := jwtrsa.Default.ParseRSAJWTToken(*ar.IDToken, kei)
	if err != nil {
		log.Err(err).Msgf("THIS ERROR IS ---NOT--- EXPECTED: %+v", err)
	}

	log.Info().Msgf("RESULT %+v", string(a))

	// "Proper" validation
	_, errUC = verifier.Verify(context.Background(), *ar.IDToken)
	if errUC != nil {
		log.Err(errUC.Err()).Msgf("THIS ERROR IS ---NOT--- EXPECTED: %+v", errUC)
	}

	// Use to sync entity
	orgClient := mycontentapiclient.New[*entity.Project](
		http.DefaultClient,
		"http://localhost:9090/project",
		nil,
	).WithToken(*ar.IDToken)

	projectSync := mycontentapiclient.Sync(orgClient, "*", sampleOrg)

	projectSync.Execute(context.Background())
}

var sampleOrg = []*entity.Project{
	{
		Ns:          "auth-sample",
		Id:          "project-1",
		Name:        "Project ggwp",
		Description: "This is an auth project",
	},
	{
		Ns:          "auth-sample",
		Id:          "project-2",
		Name:        "Project numba two",
		Description: "This is an auth project again",
	},
	{
		Ns:          "auth-sample",
		Id:          "project-3",
		Name:        "Alaamantap",
		Description: "Testing",
	},
}
