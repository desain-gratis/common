package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"time"

	authapi "github.com/desain-gratis/common/delivery/auth-api"
	mycontentapi "github.com/desain-gratis/common/delivery/mycontent-api"
	blob_gcs "github.com/desain-gratis/common/repository/blob/gcs"
	content_postgres "github.com/desain-gratis/common/repository/content/postgres"
	"github.com/desain-gratis/common/usecase/signing"
	signing_handler "github.com/desain-gratis/common/usecase/signing/handler"
	jwtrsa "github.com/desain-gratis/common/utility/secret/rsa"
	"github.com/desain-gratis/common/utility/secretkv"
	"github.com/desain-gratis/common/utility/secretkv/gsm"
	"github.com/julienschmidt/httprouter"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/desain-gratis/common/example/auth/entity"
	"github.com/desain-gratis/common/example/auth/session"
	"github.com/desain-gratis/common/example/auth/tokenbuilder"
)

func init() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Logger()
}

var (
	googleProjectID            = 146991647918
	googleOAuth2SecretJSONPath = "google-sign-in"   // GSM key to the Google Sign In JSON secret
	signingKey                 = "auth-signing-key" // GSM key to the private key used for JWT token Sign In
	tokenIssuer                = "desain.gratis"
)

func main() {
	ctx := context.Background()

	initConfig(ctx, "config/", "development")

	router := httprouter.New()

	enableApplicationAPI(router)

	address := "localhost:9090"

	server := http.Server{
		Addr:         address,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	idleConnsClosed := make(chan struct{})
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)
		<-sigint

		// We received an interrupt signal, shut down.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		log.Info().Msgf("Shutting down HTTP server..")
		if err := server.Shutdown(ctx); err != nil {
			// Error from closing listeners, or context timeout:
			log.Err(err).Msgf("HTTP server Shutdown")
		}
		log.Info().Msgf("Stopped serving new connections.")
		close(idleConnsClosed)
	}()

	log.Info().Msgf("Serving at %v..\n", address)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		// Error starting or closing listener:
		log.Fatal().Msgf("HTTP server ListendAndServe: %v", err)
	}

	<-idleConnsClosed
	log.Info().Msgf("Bye bye")

}

func enableApplicationAPI(
	router *httprouter.Router,
) {

	baseURL := "http://localhost:9090"
	privateBucketName := "data.demo.sewatenan.com"
	privateBucketBaseURL := "https://data.demo.sewatenan.com"

	pg, ok := GET_POSTGRES_SUITE_API()
	if !ok {
		log.Fatal().Msgf("Failed to obtain postgres connection")
	}

	authorizedUserRepo := content_postgres.New(pg, "gsi_authorized_user", 0)
	authorizedUserThumbnailRepo := content_postgres.New(pg, "gsi_authorized_user_thumbnail", 1)
	authorizedUserThumbnailBlobRepo := blob_gcs.New(
		privateBucketName,
		privateBucketBaseURL,
	)

	authorizedUserService := mycontentapi.New[*entity.Payload](
		authorizedUserRepo,
		baseURL+"/auth/user",
		[]string{},
	)

	authorizedUserThumbnailService := mycontentapi.NewAttachment(
		authorizedUserThumbnailRepo,
		authorizedUserThumbnailBlobRepo,
		baseURL+"/auth/thumbnail",
		[]string{"org_id", "profile_id"},
		false,                   // hide the s3 URL
		"assets/user/thumbnail", // the location in the s3 compatible bucket
		"",
	)

	secretkv.Default = gsm.NewCached(
		googleProjectID,
		map[string]map[int]gsm.CacheConfig{
			signingKey: {
				0: gsm.CacheConfig{
					PollDuration: 180 * time.Minute,
				},
			},
		},
		map[string]gsm.CacheConfig{
			// "suite-api-secret": gsm.CacheConfig{
			// 	PollDuration: 1000 * time.Minute,
			// },
		},
	)

	// Google ID token verifier
	googleIDTokenVerifier := signing_handler.NewGoogleAuth(
		signing_handler.GoogleSignInConfig{
			GoogleOAuth2SecretJSONPath: googleOAuth2SecretJSONPath,
			PollTime:                   180 * time.Second,
		},
		jwtrsa.DefaultHandler,
		secretkv.Default,
	)

	// Our own ID token signer and also verifier
	tokenSignerAndVerifier := signing_handler.New(
		signing_handler.Config{
			Issuer: tokenIssuer,
			SigningConfig: signing_handler.SigningConfig{
				Secret:   signingKey,
				ID:       "suite",
				PollTime: 1 * time.Hour,
			},
			TokenExpiryMinutes: 8 * 60,
		},
		jwtrsa.DefaultHandler,
		secretkv.Default,
	)

	// Unecessary, but allow you to see polymorphism in action
	var signer signing.Usecase = tokenSignerAndVerifier
	var verifier signing.Verifier = tokenSignerAndVerifier

	tokenBuilder := tokenbuilder.New(
		authorizedUserService.GetUsecase(),
		map[string]struct{}{
			"keenan.gebze@gmail.com": {},
		},
	)

	signingService := authapi.NewIDTokenParser(
		signer,
	)

	router.OPTIONS("/auth/admin", Empty)
	router.OPTIONS("/auth/idtoken/google", Empty)
	router.OPTIONS("/auth/idtoken", Empty)
	router.OPTIONS("/auth/keys", Empty)

	router.GET("/auth/admin", authapi.WithGoogleAuth(
		googleIDTokenVerifier,
		signingService.PublishToken(authapi.AuthParserGoogle, tokenBuilder.AdminToken),
	))
	router.GET("/auth/signin/google", authapi.WithGoogleAuth(
		googleIDTokenVerifier,
		signingService.PublishToken(authapi.AuthParserGoogle, tokenBuilder.UserToken),
	))
	router.GET("/auth/signin/debug", authapi.WithGoogleAuth(googleIDTokenVerifier, signingService.Debug))
	router.GET("/auth/signin/keys", signingService.Keys)

	// Mycontent Authorized user (admin only) endpoint
	router.OPTIONS("/auth/user", Empty)
	router.GET("/auth/user", session.WithAdminAuth(verifier, authorizedUserService.Get))
	router.POST("/auth/user", session.WithAdminAuth(verifier, authorizedUserService.Post))
	router.DELETE("/auth/user", session.WithAdminAuth(verifier, authorizedUserService.Delete))

	// Mycontent Authorized user thumbnail (admin only) endpoint
	router.OPTIONS("/auth/user/thumbnail", Empty)
	router.GET("/auth/user/thumbnail", session.WithAdminAuth(verifier, authorizedUserThumbnailService.Get))
	router.POST("/auth/user/thumbnail", session.WithAdminAuth(verifier, authorizedUserThumbnailService.Upload))
	router.DELETE("/auth/user/thumbnail", session.WithAdminAuth(verifier, authorizedUserThumbnailService.Delete))
}

func Empty(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(""))
}
