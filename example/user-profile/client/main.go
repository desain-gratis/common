package main

import (
	"context"
	"net/http"

	mycontentapiclient "github.com/desain-gratis/common/delivery/mycontent-api-client"
	"github.com/desain-gratis/common/example/user-profile/entity"
	common_entity "github.com/desain-gratis/common/types/entity"
)

func main() {
	orgClient := mycontentapiclient.New[*entity.Organization](http.DefaultClient, "localhost:9102/org", nil)
	userClient := mycontentapiclient.New[*entity.UserProfile](http.DefaultClient, "localhost:9102/org/user", []string{"org_id"})
	userPageClient := mycontentapiclient.New[*entity.UserPage](http.DefaultClient, "localhost:9102/org/user/page", []string{"org_id", "profile_id"})
	userThumbnailClient := mycontentapiclient.NewAttachment(http.DefaultClient, "localhost:9102/org/user", nil)

	orgSync := mycontentapiclient.Sync(orgClient, []*entity.Organization{})

	userSync := mycontentapiclient.Sync(userClient, []*entity.UserProfile{}).
		WithImages(userThumbnailClient, getUserProfileImage, "./images")

	userPageSync := mycontentapiclient.Sync(userPageClient, []*entity.UserPage{})

	ctx := context.Background()
	orgSync.Execute(ctx)
	userSync.Execute(ctx)
	userPageSync.Execute(ctx)
}

func getUserProfileImage(user []*entity.UserProfile) (imageRef []**common_entity.Image) {
	for idx := range user {
		imageRef = append(imageRef, &user[idx].Thumbnail_1x1)
	}
	return imageRef
}
