package main

import (
	"context"
	"net/http"
	"os"

	mycontentapiclient "github.com/desain-gratis/common/delivery/mycontent-api-client"
	"github.com/desain-gratis/common/example/user-profile/entity"
	common_entity "github.com/desain-gratis/common/types/entity"
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

	orgSync := mycontentapiclient.Sync(orgClient, "*", sampleOrg)

	userSync := mycontentapiclient.Sync(userClient, "*", sampleUser).
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

var sampleOrg []*entity.Organization = []*entity.Organization{
	{
		Id:      "pt-angin-ribut",
		OwnerId: "pt-angin-ribut",
		Name:    "PT. Angin Ribut",
	},
	{
		Id:      "mantap-corps-llc",
		OwnerId: "mantap-corps-llc",
		Name:    "Mantap Corps LLC",
	},
	{
		Id:      "sedan-berat-sdn-bhd",
		OwnerId: "sedan-berat-sdn-bhd",
		Name:    "Sedan Berat Sdn. Bhd.",
	},
	{
		Id:      "private-and-limited-pte-lte",
		OwnerId: "private-and-limited-pte-lte",
		Name:    "Private and Limited Pte. Ltd",
	},
}

var sampleUser []*entity.UserProfile = []*entity.UserProfile{
	{
		OwnerId:        "pt-angin-ribut",
		Id:             "0",
		Name:           "Budi",
		OrganizationID: "pt-angin-ribut",
		Thumbnail_1x1: &common_entity.Image{
			Id:             "pt-angin-ribut|budi.png",
			Url:            "assets/budi.png",
			ScalePx:        100,
			ScaleDirection: common_entity.SCALE_DIRECTION_HORIZONTAL,
			RatioX:         1,
			RatioY:         1,
		},
	},
	{
		OwnerId:        "pt-angin-ribut",
		Id:             "1",
		Name:           "Sarah",
		OrganizationID: "pt-angin-ribut",
		Thumbnail_1x1: &common_entity.Image{
			Id:             "pt-angin-ribut|sarah.png",
			Url:            "assets/sarah.png",
			ScalePx:        100,
			ScaleDirection: common_entity.SCALE_DIRECTION_HORIZONTAL,
			RatioX:         1,
			RatioY:         1,
		},
	},
	{
		OwnerId:        "pt-angin-ribut",
		Id:             "2",
		Name:           "Patile",
		OrganizationID: "pt-angin-ribut",
		Thumbnail_1x1: &common_entity.Image{
			Id:             "pt-angin-ribut|patile.png",
			Url:            "assets/patile.png",
			ScalePx:        100,
			ScaleDirection: common_entity.SCALE_DIRECTION_HORIZONTAL,
			RatioX:         1,
			RatioY:         1,
		},
	},
	{
		OwnerId:        "mantap-corps-llc",
		Id:             "0",
		Name:           "Mark Papandayan",
		OrganizationID: "mantap-corps-llc",
		Thumbnail_1x1: &common_entity.Image{
			Id:             "mantap-corps-llc|mark.png",
			Url:            "assets/mark.png",
			ScalePx:        100,
			ScaleDirection: common_entity.SCALE_DIRECTION_HORIZONTAL,
			RatioX:         1,
			RatioY:         1,
		},
	},
}
