package handler

import "github.com/desain-gratis/common/usecase/account"

var _ account.Usecase = &generalHandler{}

type generalHandler struct {
	*passwordHandler
}

func New(
	passwordHandler *passwordHandler,
) *generalHandler {
	return &generalHandler{
		passwordHandler: passwordHandler,
	}
}
