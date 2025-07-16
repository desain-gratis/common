package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"time"

	mycontentapi "github.com/desain-gratis/common/delivery/mycontent-api"
	blob_gcs "github.com/desain-gratis/common/delivery/mycontent-api/blob-storage/gcs"
	"github.com/desain-gratis/common/example/user-profile/entity"
	content_postgres "github.com/desain-gratis/common/repository/content/postgres"
	mycontent_base "github.com/desain-gratis/common/usecase/mycontent/base"
	"github.com/julienschmidt/httprouter"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func init() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Logger()
}

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

	organizationRepo := content_postgres.New(pg, "organization", 0) // ID overwrite-able / indemppotent (by github)
	userProfileRepo := content_postgres.New(pg, "user_profile", 1)  // ID overwrite-able / indemppotent (by github)
	userProfileThumbnailRepo := content_postgres.New(pg, "user_profile_thumbnail", 2)
	userProfileBlobRepo := blob_gcs.New(
		privateBucketName,
		privateBucketBaseURL,
	)

	organizationHandler := mycontentapi.New(
		mycontent_base.New[*entity.Organization](organizationRepo, 0),
		baseURL+"/org",
		[]string{},
	)

	userProfileHandler := mycontentapi.New(
		mycontent_base.New[*entity.UserProfile](userProfileRepo, 1),
		baseURL+"/org/user",
		[]string{"org_id"},
	)

	userThumbnailHandler := mycontentapi.NewAttachment(
		mycontent_base.NewAttachment(
			userProfileThumbnailRepo,
			2,
			userProfileBlobRepo,
			false,               // hide the s3 URL
			"assets/user/image", // the location in the s3 compatible bucket
		),
		baseURL+"/org/user/thumbnail",
		[]string{"org_id", "profile_id"},
		"",
	)

	// Organization
	router.OPTIONS("/org", Empty)
	router.GET("/org", organizationHandler.Get)
	router.POST("/org", organizationHandler.Post)
	router.DELETE("/org", organizationHandler.Delete)

	// User profile
	router.OPTIONS("/org/user", Empty)
	router.GET("/org/user", userProfileHandler.Get)
	router.POST("/org/user", userProfileHandler.Post)
	router.DELETE("/org/user", userProfileHandler.Delete)

	// User thumbnail
	router.OPTIONS("/org/user/thumbnail", Empty)
	router.GET("/org/user/thumbnail", userThumbnailHandler.Get)
	router.POST("/org/user/thumbnail", userThumbnailHandler.Upload)
	router.DELETE("/org/user/thumbnail", userThumbnailHandler.Delete)

}

func Empty(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(""))
}
