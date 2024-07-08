package handler

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"golang.org/x/sync/singleflight"

	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/usecase/signing"
	jwtrsa "github.com/desain-gratis/common/utility/secret/rsa"
	"github.com/desain-gratis/common/utility/secretkv"
)

var _ signing.Publisher = &oidcLogin{}
var _ signing.Verifier = &oidcLogin{}

type Config struct {
	// Issuer for the issuer field display
	Issuer string

	// PublishSecret contains the location of Secret for signing and it's multiple version
	SigningConfig SigningConfig

	// ProfileService connect to user's for public profile
	// instead of client hit the public profile API directly
	// since it can use internal network, it should be faster compared to client hit directly
	ProfileService string

	TokenExpiryMinutes int
}

type SigningConfig struct {
	// Secret contains the address for our signing key from secret provider
	Secret string

	// Key ID
	ID string

	// PollTime to sync the signing keys
	PollTime time.Duration
}

type SigningResponse struct {
	// ID Token is JWT Token for OpenID Connect data
	IDToken string `json:"id_token,omitempty"`

	// AccessToken is a JWT token to proof that the client is authenticated by this service
	// It is separate from ID token to avo
	AccessToken string `json:"access_token,omitempty"`

	// Profile of the user.
	Profile any `json:"profile,omitempty"`
}

type oidcLogin struct {
	config        Config
	group         *singleflight.Group
	keys          []keys
	keysLock      *sync.Mutex
	rsaStore      jwtrsa.Provider
	secretkvStore secretkv.Provider
}

type keys struct {
	signing.Keys
	cacheUpdateTime time.Time
	createdAt       time.Time
}

// New create new OIDC Login usecase
func New(
	config Config,
	rsaStore jwtrsa.Provider,
	secretkvStore secretkv.Provider,
) *oidcLogin {
	s := &oidcLogin{
		config:        config,
		group:         &singleflight.Group{},
		keysLock:      &sync.Mutex{},
		keys:          make([]keys, 0),
		rsaStore:      rsaStore,
		secretkvStore: secretkvStore,
	}

	go s.updateSigningKeys(context.Background(), s.config.SigningConfig.Secret)

	return s
}

func (s *oidcLogin) Sign(ctx context.Context, claim []byte, expire time.Time) (token string, errUC *types.CommonError) {
	keys, errUC := s.getKeys(ctx)
	if errUC != nil || len(keys) == 0 {
		return "", &types.CommonError{
			Errors: []types.Error{
				{
					Code:     "FAILED_TO_GET_KEYS",
					HTTPCode: http.StatusInternalServerError,
					Message:  "Internal server error",
				},
			},
		}
	}

	// tokenExpiry := time.Now().Add(time.Duration(s.config.TokenExpiryMinutes) * time.Minute)
	token, err := s.rsaStore.BuildRSAJWTToken(claim, expire, keys[0].KeyID)
	if err != nil {
		return "", &types.CommonError{
			Errors: []types.Error{
				{
					Code:     "GENERATE_TOKEN_FAILED",
					HTTPCode: http.StatusInternalServerError,
					Message:  "Failed to generate token",
				},
			},
		}
	}
	return token, nil
}

func (s *oidcLogin) Keys(ctx context.Context) ([]signing.Keys, *types.CommonError) {
	result, err := s.getKeys(ctx)
	return convertToOIDCKeys(result), err
}

func (s *oidcLogin) getKeys(ctx context.Context) ([]keys, *types.CommonError) {
	if len(s.keys) > 0 {
		if time.Now().Sub(s.keys[0].cacheUpdateTime) < s.config.SigningConfig.PollTime {
			return s.keys, nil
		}
	}
	return s.updateSigningKeys(ctx, s.config.SigningConfig.Secret)
}

func (s *oidcLogin) updateSigningKeys(ctx context.Context, key string) ([]keys, *types.CommonError) {
	res := s.group.DoChan(s.config.SigningConfig.Secret, func() (interface{}, error) {
		payloads, err := s.secretkvStore.List(context.Background(), s.config.SigningConfig.Secret)
		if err != nil {
			log.Err(err).Msgf("Failed to get sigining secret")
			return nil, err
		}
		var result []keys

		updateTime := time.Now()
		for _, payload := range payloads {
			// KEY_ID & version
			KEY_ID := s.config.SigningConfig.ID + "-v" + strconv.Itoa(payload.Version)
			err := s.rsaStore.StorePrivateKey(KEY_ID, string(payload.Payload))
			if err != nil {
				log.Err(err).Msgf("Failed storing ECSDA secret `%v`", KEY_ID)
				continue
			}
			pub, ok, err := s.rsaStore.GetPublicKey(KEY_ID)
			if err != nil {
				log.Err(err).Msgf("Failed parsing ECSDA secret `%v`", KEY_ID)
				continue
			}
			if !ok {
				// not found
				log.Err(err).Msgf("Not found `%v`", KEY_ID)
				continue
			}
			result = append(result, keys{
				Keys: signing.Keys{
					CreatedAt: payload.CreatedAt.Format(time.RFC3339),
					KeyID:     KEY_ID,
					Key:       string(pub),
				},
				cacheUpdateTime: updateTime,
				createdAt:       payload.CreatedAt,
			})
		}

		sort.Slice(result, func(i, j int) bool {
			return result[i].createdAt.After(result[j].createdAt)
		})

		func() {
			s.keysLock.Lock()
			defer s.keysLock.Unlock()
			s.keys = result
		}()

		return result, nil
	})

	select {
	case srResult := <-res:
		keys, _ := srResult.Val.([]keys)
		var err *types.CommonError
		if srResult.Err != nil {
			err = &types.CommonError{
				Errors: []types.Error{
					{
						HTTPCode: http.StatusInternalServerError,
						Code:     "SERVER_CACHING_ERROR",
						Message:  "Failed to get keys. Please retry again",
					},
				},
			}
		}
		return keys, err
	case _ = <-ctx.Done():
		return nil, &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusInternalServerError,
					Code:     "TIMED_OUT",
					Message:  "Server timed out",
				},
			},
		}
	}
}

func convertToOIDCKeys(keys []keys) []signing.Keys {
	result := make([]signing.Keys, 0, len(keys))
	for _, v := range keys {
		result = append(result, v.Keys)
	}
	return result
}

func (s *oidcLogin) Verify(ctx context.Context, token string) (claim []byte, errUC *types.CommonError) {
	if len(s.keys) == 0 {
		log.Err(errors.New("empty public key in verifier")).Msgf("Empty public keys: `%v`", s.config.SigningConfig.Secret)
		errUC = &types.CommonError{
			Errors: []types.Error{
				{HTTPCode: http.StatusInternalServerError, Code: "FAILED_TO_PARSE_TOKEN", Message: "Failed to parse token"},
			},
		}
		return nil, errUC
	}

	// TODO: handle keys versioning. currenly will only support the last version
	payload, err := s.rsaStore.ParseRSAJWTToken(token, s.keys[0].KeyID)
	if err != nil {
		return claim, &types.CommonError{
			Errors: []types.Error{
				{Code: "INVALID_AUTHORIZATION_TOKEN", HTTPCode: http.StatusBadRequest, Message: "Invalid authorization token"},
			},
		}
	}

	return payload, nil
}
