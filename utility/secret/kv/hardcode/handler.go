package hardcode

import (
	"sync"
)

type defaultHandler struct {
	publicKeysLock *sync.Mutex
	publicKeys     map[string]string
}

func New() *defaultHandler {
	return &defaultHandler{
		publicKeysLock: &sync.Mutex{},
		publicKeys:     make(map[string]string),
	}
}

func (d *defaultHandler) Store(keyID string, secret string) (err error) {
	d.publicKeysLock.Lock()
	defer d.publicKeysLock.Unlock()

	d.publicKeys[keyID] = secret
	return nil
}

func (d *defaultHandler) Get(keyID string) (secret string, ok bool, err error) {
	d.publicKeysLock.Lock()
	defer d.publicKeysLock.Unlock()

	secret, ok = d.publicKeys[keyID]
	return
}
