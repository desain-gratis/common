package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"
	"net/http"

	types "github.com/desain-gratis/common/types/http"
	"github.com/rs/zerolog/log"
)

func Encrypt(key []byte, value []byte) (chipertext []byte, iv []byte, errUC *types.CommonError) {
	block, err := aes.NewCipher(key)
	if err != nil {
		log.Err(err).Msgf("Failed to build chiper")
		return nil, nil, &types.CommonError{
			Errors: []types.Error{
				{Code: "SERVER_ERROR", HTTPCode: http.StatusInternalServerError, Message: "Failed to build chiper"},
			},
		}
	}

	// Never use more than 2^32 random nonces with a given key because of the risk of a repeat.
	nonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		log.Err(err).Msgf("Failed to read random for nonce")
		return nil, nil, &types.CommonError{
			Errors: []types.Error{
				{Code: "SERVER_ERROR", HTTPCode: http.StatusInternalServerError, Message: "Failed to read random"},
			},
		}
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, &types.CommonError{
			Errors: []types.Error{
				{Code: "SERVER_ERROR", HTTPCode: http.StatusInternalServerError, Message: "Failed to build chiper operation"},
			},
		}
	}

	ciphertext := aesgcm.Seal(nil, nonce, value, nil)

	return ciphertext, nonce, nil
}

func Decrypt(key []byte, iv []byte, chipertext []byte) (result []byte, errUC *types.CommonError) {
	block, err := aes.NewCipher(key)
	if err != nil {
		log.Err(err).Msgf("Failed to build chiper")
		return nil, &types.CommonError{
			Errors: []types.Error{
				{Code: "SERVER_ERROR", HTTPCode: http.StatusInternalServerError, Message: "Failed to set data"},
			},
		}
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		log.Err(err).Msgf("Failed to build aes gcm")
		return nil, &types.CommonError{
			Errors: []types.Error{
				{Code: "SERVER_ERROR", HTTPCode: http.StatusInternalServerError, Message: "Failed to set data"},
			},
		}
	}

	plaintext, err := aesgcm.Open(nil, iv, chipertext, nil)
	if err != nil {
		log.Err(err).Msgf("Failed to build aes gcm")
		return nil, &types.CommonError{
			Errors: []types.Error{
				{Code: "SERVER_ERROR", HTTPCode: http.StatusInternalServerError, Message: "Failed to set data"},
			},
		}
	}

	return plaintext, nil
}
