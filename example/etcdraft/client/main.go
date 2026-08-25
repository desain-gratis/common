package main

import (
	"context"
	"net/url"
	"time"

	"github.com/rs/zerolog/log"

	contentsync "github.com/desain-gratis/common/delivery/mycontent-api-client"
	"github.com/desain-gratis/common/example/etcdraft/entity"
)

var data = []*entity.UserProfile{
	{
		Id:             "maknyus",
		Ns:             "sewa",
		Url:            "",
		Name:           "Keenan",
		CreatedAt:      time.Now().Format(time.RFC3339),
		OrganizationID: "cihuy",
	},
	{
		Id:             "2",
		Ns:             "sewa",
		Url:            "",
		Name:           "Zotong",
		CreatedAt:      time.Now().Format(time.RFC3339),
		OrganizationID: "cihuy",
	},
}

func main() {
	u, err := url.Parse("http://localhost:9001/user")
	if err != nil {
		log.Fatal().Msgf("err: %v", err)
	}
	buildSync := contentsync.Builder[*entity.UserProfile](u).
		WithNamespace("*").
		WithData(data)

	err = buildSync.Build().Execute(context.Background())
	if err != nil {
		log.Panic().Msgf("failed to execute: %v", err)
	}
}
