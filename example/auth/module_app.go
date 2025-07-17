package main

import (
	"net/http"
	"os"

	authapi "github.com/desain-gratis/common/delivery/auth-api"
	idtokensigner "github.com/desain-gratis/common/delivery/auth-api/idtoken-signer"
	idtokenverifier "github.com/desain-gratis/common/delivery/auth-api/idtoken-verifier"
	mycontentapi "github.com/desain-gratis/common/delivery/mycontent-api"
	blob_gcs "github.com/desain-gratis/common/delivery/mycontent-api/storage/blob/gcs"
	content_postgres "github.com/desain-gratis/common/delivery/mycontent-api/storage/content/postgres"
	"github.com/desain-gratis/common/example/auth/entity"
	"github.com/desain-gratis/common/example/auth/plugin"
	mycontent_base "github.com/desain-gratis/common/usecase/mycontent/base"
	"github.com/julienschmidt/httprouter"
	"github.com/rs/zerolog/log"
)

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
	authorizedUserRepo := content_postgres.New(pg, "authorized_user", 0)
	authorizedUserThumbnailRepo := content_postgres.New(pg, "authorized_user_thumbnail", 1)
	authorizedUserThumbnailBlobRepo := blob_gcs.New(
		privateBucketName,
		privateBucketBaseURL,
	)
	projectRepo := content_postgres.New(pg, "project", 0)

	// Initialize usecase logic
	userUsecase := mycontent_base.New[*entity.UserAuthorization](
		authorizedUserRepo,
		0,
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

	// Our own ID token signer and also verifier
	tokenSignerAndVerifier := idtokensigner.New(
		"auth-example",
		map[string]string{
			"key-v1": "th1s 1s 4 s3creet. shhhh!1!!",
		},
		"key-v1",
	)

	gsiAuth := idtokenverifier.GSIAuth(CONFIG.GetString("gsi.client_id"))
	appAuth := idtokenverifier.AppAuth(tokenSignerAndVerifier, plugin.AuthCtxKey, plugin.ParseToken)

	adminTokenExpiryMinutes := 1 * 30
	userTokenExpiryMinutes := 8 * 60
	adminEmailGSI := os.Getenv("ADMIN_EMAIL")
	if adminEmailGSI == "" {
		log.Panic().Msgf("please specify admin email in ADMIN_EMAIL env")
	}

	adminTokenBuilder := plugin.AdminAuthLogic(map[string]struct{}{adminEmailGSI: {}}, adminTokenExpiryMinutes)
	userTokenBuilder := plugin.NewUserAuthLogic(nil, userTokenExpiryMinutes)

	// --- Initialize HTTP services handler ---

	// Service for user authorization management
	userAuthService := mycontentapi.New(
		plugin.MyContentWithAuth(userUsecase), // notice, authorization here
		baseURL+"/auth/user",
		[]string{},
	)

	// Thumbnail for user authorization
	userAuthThumbnailService := mycontentapi.NewAttachment(
		authorizedUserThumbnailUsecase,
		baseURL+"/auth/user/thumbnail",
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
	router.GET("/auth/admin", gsiAuth(authapi.GetToken(adminTokenBuilder, tokenSignerAndVerifier)))
	router.GET("/auth/signin", gsiAuth(authapi.GetToken(userTokenBuilder, tokenSignerAndVerifier)))

	// Debug app token and verify using public key
	tokenAPI := authapi.NewTokenAPI(tokenSignerAndVerifier, plugin.ParseToken)

	router.GET("/auth/debug", appAuth(tokenAPI.Debug))
	router.GET("/auth/keys", appAuth(tokenAPI.Keys))

	// Mycontent authorized user (admin only) endpoint
	router.OPTIONS("/auth/user", Empty)
	router.GET("/auth/user", appAuth(plugin.AdminOnly(userAuthService.Get)))
	router.POST("/auth/user", appAuth(userAuthService.Post))
	router.DELETE("/auth/user", appAuth(userAuthService.Delete))

	// Mycontent Authorized user thumbnail (admin only) endpoint
	router.OPTIONS("/auth/user/thumbnail", Empty)
	router.GET("/auth/user/thumbnail", appAuth(userAuthThumbnailService.Get))
	router.POST("/auth/user/thumbnail", appAuth(userAuthThumbnailService.Upload))
	router.DELETE("/auth/user/thumbnail", appAuth(userAuthThumbnailService.Delete))

	// Mycontent sample entity
	router.OPTIONS("/project", Empty)
	router.GET("/project", appAuth(projectService.Get))
	router.POST("/project", appAuth(projectService.Post))
	router.DELETE("/project", appAuth(projectService.Delete))
}

func Empty(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(""))
}
