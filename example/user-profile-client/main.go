package main

import (
	"context"
	"net/http"
	"os"

	mycontentapiclient "github.com/desain-gratis/common/delivery/mycontent-api-client"
	"github.com/desain-gratis/common/example/user-profile/entity"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func init() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Logger()
}

func main() {
	orgClient := mycontentapiclient.New[*entity.Organization](http.DefaultClient, "http://localhost:9090/org", nil)
	userClient := mycontentapiclient.New[*entity.UserProfile](http.DefaultClient, "http://localhost:9090/org/user", []string{"org_id"})
	userThumbnailClient := mycontentapiclient.NewAttachment(http.DefaultClient, "http://localhost:9090/org/user/thumbnail", []string{"org_id", "profile_id"})

	orgSync := mycontentapiclient.Sync(orgClient, "*", sampleOrg, mycontentapiclient.OptionalConfig{})

	userSync := mycontentapiclient.Sync(userClient, "*", sampleUser, mycontentapiclient.OptionalConfig{}).
		WithImages(userThumbnailClient, getUserProfileImage, ".") // upload from local URL, with . root directory

	ctx := context.Background()
	orgSync.Execute(ctx)
	userSync.Execute(ctx)
}

func getUserProfileImage(users []*entity.UserProfile) (imageRefs []mycontentapiclient.ImageContext[*entity.UserProfile]) {
	for idx := range users {
		imageRefs = append(imageRefs, mycontentapiclient.ImageContext[*entity.UserProfile]{
			Image: &users[idx].Thumbnail_1x1,
			Base:  users[idx],
		})
	}
	return imageRefs
}
