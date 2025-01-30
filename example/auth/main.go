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
	mycontent_base "github.com/desain-gratis/common/usecase/mycontent/base"
	"github.com/desain-gratis/common/usecase/signing"
	signing_handler "github.com/desain-gratis/common/usecase/signing/handler"
	jwtrsa "github.com/desain-gratis/common/utility/secret/rsa"
	"github.com/desain-gratis/common/utility/secretkv"
	"github.com/desain-gratis/common/utility/secretkv/gsm"
	"github.com/julienschmidt/httprouter"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/desain-gratis/common/example/auth/entity"
	"github.com/desain-gratis/common/example/auth/plugin"
)

func init() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Logger()
}

var (
	googleProjectID = 1015170299395
	signingKey      = "auth-signing-key" // GSM key to the private key used for JWT token Sign In
	tokenIssuer     = "https://desain.gratis"
)

func main() {
	ctx := context.Background()

	// init secret connection
	secretkv.Default = gsm.NewCached(
		googleProjectID,
		map[string]map[int]gsm.CacheConfig{
			signingKey: {
				0: gsm.CacheConfig{
					PollDuration: 180 * time.Minute,
				},
			},
		},
		map[string]gsm.CacheConfig{},
	)

	// init config
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

	// Initialize datasources for mycontent
	authorizedUserRepo := content_postgres.New(pg, "gsi_authorized_user", 0)
	authorizedUserThumbnailRepo := content_postgres.New(pg, "gsi_authorized_user_thumbnail", 1)
	projectRepo := content_postgres.New(pg, "project", 0)
	authorizedUserThumbnailBlobRepo := blob_gcs.New(
		privateBucketName,
		privateBucketBaseURL,
	)

	// Initialize usecase logic
	authorizedUserUsecase := plugin.MyContentWithAuth(
		mycontent_base.New[*entity.Payload](
			authorizedUserRepo,
			0,
		),
	)

	authorizedUserThumbnailUsecase := plugin.MyContentAttachmentWithAuth(
		mycontent_base.NewAttachment(
			authorizedUserThumbnailRepo,
			1,
			authorizedUserThumbnailBlobRepo,
			false,
			"assets/user/thumbnail",
		),
	)

	projectUsecase := plugin.MyContentWithAuth(
		mycontent_base.New[*entity.Project](
			projectRepo,
			0,
		),
	)

	// Plugin to publish token
	tokenBuilder := plugin.TokenPublisher(
		authorizedUserUsecase,
		map[string]struct{}{
			"keenan.gebze@gmail.com": {},
			"desain-gratis-developer@langsunglelang.iam.gserviceaccount.com": {},
		},
	)

	// Google ID token verifier
	googleVerifier := signing_handler.NewGoogleAuth(CONFIG.GetString("gsi.client_id"))

	// Our own ID token signer and also verifier
	tokenSignerAndVerifier := signing_handler.New(
		signing_handler.Config{
			Issuer: tokenIssuer,
			SigningConfig: signing_handler.SigningConfig{
				Secret:   signingKey,
				ID:       "desain-gratis",
				PollTime: 1 * time.Hour,
			},
			TokenExpiryMinutes: 8 * 60,
		},
		jwtrsa.Default,
		secretkv.Default,
	)

	// Unecessary, but allow you to see polymorphism in action
	var appTokenSigner signing.Signer = tokenSignerAndVerifier
	var appTokenVerifier signing.Verifier = tokenSignerAndVerifier

	// Exchange valid Google ID token with Application token
	googleauth := authapi.NewTokenExchanger(googleVerifier, appTokenSigner)

	// Token usage
	appauth := plugin.AuthProvider(appTokenVerifier, appTokenSigner)

	// --- Initialize HTTP services handler ---

	// Service for user authorization management
	userAuthService := mycontentapi.New(
		authorizedUserUsecase,
		baseURL+"/auth/user",
		[]string{},
	)

	// Thumbnail for user authorization
	userAuthThumbnailService := mycontentapi.NewAttachment(
		authorizedUserThumbnailUsecase,
		baseURL+"/auth/thumbnail",
		[]string{"org_id", "profile_id"},
		"",
	)

	// Sample API
	projectService := mycontentapi.New(
		projectUsecase,
		baseURL+"/project",
		[]string{},
	)

	// Http router

	router.OPTIONS("/auth/admin", Empty)
	router.OPTIONS("/auth/signin", Empty)
	router.OPTIONS("/auth/debug", Empty)
	router.OPTIONS("/auth/keys", Empty)

	// Sign-in as admin, sign-in as user
	router.GET("/auth/admin", googleauth.ExchangeToken(tokenBuilder.AdminOnlyToken))
	router.GET("/auth/signin", googleauth.ExchangeToken(tokenBuilder.UserToken))

	// Debug app token and verify using public key
	router.GET("/auth/debug", appauth.Debug)
	router.GET("/auth/keys", appauth.Keys)

	// Mycontent authorized user (admin only) endpoint
	router.OPTIONS("/auth/user", Empty)
	router.GET("/auth/user", appauth.AdminOnly(userAuthService.Get))
	router.POST("/auth/user", appauth.AdminOnly(userAuthService.Post))
	router.DELETE("/auth/user", appauth.AdminOnly(userAuthService.Delete))

	// Mycontent Authorized user thumbnail (admin only) endpoint
	router.OPTIONS("/auth/user/thumbnail", Empty)
	router.GET("/auth/user/thumbnail", appauth.AdminOnly(userAuthThumbnailService.Get))
	router.POST("/auth/user/thumbnail", appauth.AdminOnly(userAuthThumbnailService.Upload))
	router.DELETE("/auth/user/thumbnail", appauth.AdminOnly(userAuthThumbnailService.Delete))

	// Mycontent sample entity
	router.OPTIONS("/project", Empty)
	router.GET("/project", appauth.User(projectService.Get))
	router.POST("/project", appauth.User(projectService.Post))
	router.DELETE("/project", appauth.User(projectService.Delete))
}

func Empty(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(""))
}
