package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"golang.org/x/sync/singleflight"
	"google.golang.org/api/idtoken"

	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/usecase/signing"
	jwtrsa "github.com/desain-gratis/common/utility/secret/rsa"
	"github.com/desain-gratis/common/utility/secretkv"
)

var _ signing.VerifierOf[idtoken.Payload] = &googleVerifier{}

type GoogleOAuth2Secret struct {
	SyncTime time.Time
	Web      GoogleOAuthSecretWeb `json:"web"`
}

type GoogleOAuthSecretWeb struct {
	ClientID                string   `json:"client_id"`
	ProjectID               string   `json:"project_id"`
	AuthURI                 string   `json:"auth_uri"`
	TokenURI                string   `json:"token_uri"`
	AuthProviderX509CertUrl string   `json:"auth_provider_x509_cert_url"`
	ClientSecret            string   `json:"client_secret"`
	RedirectURIs            []string `json:"redirect_uris"`
	JavascriptOrigin        []string `json:"javascript_origins"`
}

type GoogleSignInConfig struct {
	// The location for google OAuth v2 JSON file in the secret-provider
	GoogleOAuth2SecretJSONPath string
	PollTime                   time.Duration
}

type googleVerifier struct {
	config GoogleSignInConfig
	group  *singleflight.Group

	clientSecretCache     *GoogleOAuth2Secret
	clientSecretCacheLock *sync.Mutex
	rsaProvider           jwtrsa.Provider
	secretProvider        secretkv.Provider
}

func NewGoogleAuth(config GoogleSignInConfig, rsaProvider jwtrsa.Provider, secretProvider secretkv.Provider) *googleVerifier {
	return &googleVerifier{
		config:                config,
		group:                 &singleflight.Group{},
		clientSecretCache:     &GoogleOAuth2Secret{},
		clientSecretCacheLock: &sync.Mutex{},
		rsaProvider:           rsaProvider,
		secretProvider:        secretProvider,
	}
}

func (g *googleVerifier) VerifyAs(ctx context.Context, token string) (*idtoken.Payload, *types.CommonError) {
	// check cache
	if time.Now().Sub(g.clientSecretCache.SyncTime) > g.config.PollTime {
		secret, errUC := g.updateSigningKeys(ctx)
		if errUC != nil || secret == nil {
			return nil, errUC
		}
	}

	result, err := idtoken.Validate(ctx, token, g.clientSecretCache.Web.ClientID)
	if err != nil {
		return nil, &types.CommonError{
			Errors: []types.Error{
				{Code: "UNAUTHORIZED", HTTPCode: http.StatusBadRequest, Message: "Unauthorized. Please specify correct token."},
			},
		}
	}

	return result, nil
}

func (g *googleVerifier) updateSigningKeys(ctx context.Context) (*GoogleOAuth2Secret, *types.CommonError) {
	res := g.group.DoChan(g.config.GoogleOAuth2SecretJSONPath, func() (interface{}, error) {
		payload, err := g.secretProvider.Get(context.Background(), g.config.GoogleOAuth2SecretJSONPath, 0)
		if err != nil {
			log.Err(err).Msgf("Failed to get sigining secret")
			return nil, err
		}
		var result GoogleOAuth2Secret
		json.Unmarshal(payload.Payload, &result)

		result.SyncTime = time.Now()

		func() {
			g.clientSecretCacheLock.Lock()
			defer g.clientSecretCacheLock.Unlock()
			g.clientSecretCache = &result
		}()

		return result, nil
	})

	select {
	case srResult := <-res:
		keys, _ := srResult.Val.(GoogleOAuth2Secret)
		var err *types.CommonError
		if srResult.Err != nil {
			err = &types.CommonError{
				Errors: []types.Error{
					{
						HTTPCode: http.StatusInternalServerError,
						Code:     "SERVER_CACHING_ERROR",
						Message:  "Failed to get keys. Please retry again",
					},
				},
			}
		}
		return &keys, err
	case _ = <-ctx.Done():
		return nil, &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusInternalServerError,
					Code:     "TIMED_OUT",
					Message:  "Server timed out",
				},
			},
		}
	}
}
