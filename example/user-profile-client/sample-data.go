package main

import (
	"github.com/desain-gratis/common/example/user-profile/entity"
	common_entity "github.com/desain-gratis/common/types/entity"
)

var sampleOrg []*entity.Organization = []*entity.Organization{
	{
		Ns:   "pt-angin-ribut",
		Id:   "pt-angin-ribut",
		Name: "PT. Angin Ribut",
	},
	{
		Ns:   "mantap-corps-llc",
		Id:   "mantap-corps-llc",
		Name: "Mantap Corps LLC",
	},
	{
		Ns:   "sedan-berat-sdn-bhd",
		Id:   "sedan-berat-sdn-bhd",
		Name: "Sedan Berat Sdn. Bhd.",
	},
	{
		Ns:   "private-and-limited-pte-lte",
		Id:   "private-and-limited-pte-lte",
		Name: "Private and Limited Pte. Ltd",
	},
}

var _sampleUser []*entity.UserProfile = []*entity.UserProfile{}
var sampleUser []*entity.UserProfile = []*entity.UserProfile{
	{
		Ns:             "pt-angin-ribut",
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
		Ns:             "pt-angin-ribut",
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
		Ns:             "pt-angin-ribut",
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
		Ns:             "mantap-corps-llc",
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
