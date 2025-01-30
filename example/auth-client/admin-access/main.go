package main

import (
	"context"
	"net/http"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	authclient "github.com/desain-gratis/common/delivery/auth-api-client"
	mycontentapiclient "github.com/desain-gratis/common/delivery/mycontent-api-client"
	"github.com/desain-gratis/common/example/auth/entity"
	common_entity "github.com/desain-gratis/common/types/entity"
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
	//
	idToken := os.Getenv("GOOGLE_ID_TOKEN")
	if idToken == "" {
		log.Fatal().Msgf("Please specify GOOGLE_ID_TOKEN environment variables with google id token")
	}

	// ADMIN SIGN IN
	aca := authclient.New("http://localhost:9090/auth/admin")
	ar, errUC := aca.SignIn(context.Background(), idToken)
	if errUC != nil {
		log.Fatal().Msgf("Err get google ID! %v", errUC)
	}

	log.Info().Msgf("Token: %+v", *ar.IDToken)

	// Use to sync "project"
	projectClient := mycontentapiclient.New[*entity.Project](
		http.DefaultClient,
		"http://localhost:9090/project",
		nil,
	)

	// As an admin, you can sync projects using special all namespace "*"
	projectSync := mycontentapiclient.Sync(projectClient, "*", sampleOrg, mycontentapiclient.OptionalConfig{
		AuthorizationToken: *ar.IDToken,
	})

	errUC = projectSync.Execute(context.Background())
	if errUC != nil {
		log.Fatal().Err(errUC.Err()).Msgf("ERROR NOT EXPECTED: %v", errUC)
	}

	// As an admin, you can sync application's authorized users!

	// Use to sync "project"
	authorizedUserClient := mycontentapiclient.New[*entity.UserAuthorization](
		http.DefaultClient,
		"http://localhost:9090/auth/user",
		nil,
	)

	// Sync user picture
	authorizedUserImagesClient := mycontentapiclient.NewAttachment(
		http.DefaultClient,
		"http://localhost:9090/auth/user/thumbnail",
		nil,
	)

	authorizedUserSync := mycontentapiclient.Sync(authorizedUserClient, "*", sampleUsers, mycontentapiclient.OptionalConfig{
		AuthorizationToken: *ar.IDToken,
	}).WithImages(authorizedUserImagesClient, extractAuthorizedUserImages, ".")

	errUC = authorizedUserSync.Execute(context.Background())
	if errUC != nil {
		log.Fatal().Err(errUC.Err()).Msgf("ERROR: %v", errUC)
	}

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
	{
		Ns:          "new-project",
		Id:          "new-project",
		Name:        "New Project",
		Description: "This project is new and GGWP",
	},
}

var sampleUsers = []*entity.UserAuthorization{
	{
		// the same admin email can behave as ordinary user if the token is not admin
		Id: "desain-gratis-developer@langsunglelang.iam.gserviceaccount.com",
		DefaultProfile: entity.UserProfile{
			DisplayName: "Keenan G",
			Description: "User of the application",
			Avatar1x1: &common_entity.Image{
				Id:      "default-profile-image",
				Url:     "avatar.png",
				ScalePx: 100,
				RatioX:  1,
				RatioY:  1,
			},
			Background3x1: &common_entity.Image{
				Id:      "default-profile-background",
				Url:     "background.png",
				ScalePx: 300,
				RatioX:  3,
				RatioY:  1,
			},
		},
		Ns:             "root",
		DefaultProject: "new-project",
		Authorization: map[string]entity.Authorization{
			"new-project": {
				UserGroupID2: "orkes-ui",
				UserGroupID:  map[string]struct{}{}, // backward compat
				UiAndApiPermission: map[string]bool{
					"i-can-open-this-page": true,
				},
			},
		},
	},
}

func extractAuthorizedUserImages(users []*entity.UserAuthorization) (images []mycontentapiclient.ImageContext[*entity.UserAuthorization]) {
	// prepare to upload user avatar and user background
	for idx, _ := range users {
		images = append(images, mycontentapiclient.ImageContext[*entity.UserAuthorization]{
			Base:  users[idx],
			Image: &users[idx].DefaultProfile.Avatar1x1,
		})

		// get the user background
		images = append(images, mycontentapiclient.ImageContext[*entity.UserAuthorization]{
			Base:  users[idx],
			Image: &users[idx].DefaultProfile.Background3x1,
		})
	}
	return images
}
