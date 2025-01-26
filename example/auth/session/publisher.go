package session

import (
	"github.com/desain-gratis/common/usecase/signing"
	"github.com/julienschmidt/httprouter"
)

func PublishToken(uc signing.Verifier, handle httprouter.Handle) httprouter.Handle {
	return nil
}
