package main

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"time"

	mycontentapi "github.com/desain-gratis/common/delivery/mycontent-api"
	"github.com/desain-gratis/common/example/user-profile/types"
	blob_gcs "github.com/desain-gratis/common/repository/blob/gcs"
	content_postgres "github.com/desain-gratis/common/repository/content/postgres"
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

	baseURL := "http://private-server:9090"
	privateBucketName := "data.demo.sewatenan.com"
	privateBucketBaseURL := "https://data.demo.sewatenan.com"

	pg, ok := GET_POSTGRES_SUITE_API()
	if !ok {
		log.Fatal().Msgf("Failed to obtain postgres connection")
	}

	organizationRepo := content_postgres.New(pg, "organization") // ID overwrite-able / indemppotent (by github)
	userProfileRepo := content_postgres.New(pg, "user_profile")  // ID overwrite-able / indemppotent (by github)
	userProfileThumbnailRepo := content_postgres.New(pg, "user_profile_thumbnail")
	userProfileBlobRepo := blob_gcs.New(
		privateBucketName,
		privateBucketBaseURL,
	)
	userPageRepo := content_postgres.New(pg, "user_profile_thumbnail")

	organizationHandler := mycontentapi.New(
		organizationRepo,
		types.ValidateOrganization,
		func(v url.Values) []string {
			// because user profile is the "base", can only accessed by ID or User ID.
			return make([]string, 0)
		},
		func(url, userID string, refID []string, ID string) string {
			return baseURL + "/user?user_id=" + userID + "&id=" + ID
		},
	)

	userProfileHandler := mycontentapi.New(
		userProfileRepo,
		types.ValidateUserProfile,
		func(v url.Values) []string {
			// allow user profile to be queried by org_id
			return []string{v.Get("org_id")}
		},
		func(url, userID string, refID []string, ID string) string {
			if len(refID) == 0 {
				log.Error().Msgf("This should not happen")
				return baseURL + "/user?user_id=" + userID + "&id=" + ID
			}
			return baseURL + "/user?user_id=" + userID + "&org_id=" + refID[0] + "&id=" + ID
		},
	)

	userThumbnailHandler := mycontentapi.NewAttachment(
		userProfileThumbnailRepo,
		userProfileBlobRepo,
		func(v url.Values) []string {
			return []string{v.Get("user_id")} // user_id acts as ref_id as well
		},
		false,               // hide the s3 URL
		"assets/user/image", // the location in the s3 compatible bucket
		func(url, userID string, refID []string, ID string) string {
			return baseURL + "/user/thumbnail?user_id=" + userID + "&id=" + ID
		},
		"",
	)

	userPageHandler := mycontentapi.New(
		userPageRepo,
		types.ValidateUserPage,
		func(v url.Values) []string {
			// allow this page to be queried by org_id and profile_id
			return []string{v.Get("org_id"), v.Get("profile_id")}
		},
		func(url, userID string, refID []string, ID string) string {
			if len(refID) != 2 {
				log.Error().Msgf("This should not happen")
				return baseURL + "/user?user_id=" + userID + "&id=" + ID
			}
			return baseURL + "/user?user_id=" + userID + "&org_id=" + refID[0] + "&profile_id=" + refID[1] + "&id=" + ID
		},
	)

	// Organization
	router.OPTIONS("/org", Empty)
	router.GET("/org", organizationHandler.Get)
	router.PUT("/org", organizationHandler.Put)
	router.DELETE("/org", organizationHandler.Delete)

	// User profile
	router.OPTIONS("/org/user", Empty)
	router.GET("/org/user", userProfileHandler.Get)
	router.PUT("/org/user", userProfileHandler.Put)
	router.DELETE("/org/user", userProfileHandler.Delete)

	// User thumbnail
	router.OPTIONS("/org/user/thumbnail", Empty)
	router.GET("/org/user/thumbnail", userThumbnailHandler.Get)
	router.PUT("/org/user/thumbnail", userThumbnailHandler.Upload)
	router.DELETE("/org/user/thumbnail", userThumbnailHandler.Delete)

	// User page
	router.OPTIONS("/org/user/page", Empty)
	router.GET("/org/user/page", userPageHandler.Get)
	router.PUT("/org/user/page", userPageHandler.Put)
	router.DELETE("/org/user/page", userPageHandler.Delete)
}

func Empty(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(""))
}
