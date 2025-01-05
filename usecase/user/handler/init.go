package handler

import (
	m "github.com/desain-gratis/common/repository"
	pg "github.com/desain-gratis/common/repository/common"
	uc "github.com/desain-gratis/common/usecase/user"
)

type usecase struct {
	userPGRepo pg.Repository[m.AuthorizedUser]
}

func New(userPGRepo pg.Repository[m.AuthorizedUser]) uc.UseCase {
	return &usecase{
		userPGRepo: userPGRepo,
	}
}
