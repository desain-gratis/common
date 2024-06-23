package hardcode

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"sync"
	"time"

	"github.com/golang-jwt/jwt"
)

var (
	ErrInvalidSigningMethod  = errors.New("invalid signing method")
	ErrKeyIdentifierNotFound = errors.New("key identifier not found")
	ErrKeyIdentifierEmpty    = errors.New("key identifier empty")
	ErrInvalidToken          = errors.New("invalid token")
	ErrInvalidClaim          = errors.New("invalid claim")
	ErrInvalidPayload        = errors.New("invalid payload")
)

const (
	ISS           = "account-service"
	KID_HEADER    = "kid"
	PAYLOAD_CLAIM = "payload"
)

type CustomClaim struct {
	jwt.StandardClaims
	Payload string `json:"payload"`
}

type defaultHandler struct {
	issuer          string
	privateKeysLock *sync.Mutex
	privateKeys     map[string]*ecdsa.PrivateKey
	publicKeysLock  *sync.Mutex
	publicKeys      map[string]*ecdsa.PublicKey
}

func New(issuer string) *defaultHandler {
	return &defaultHandler{
		issuer:          issuer,
		privateKeys:     make(map[string]*ecdsa.PrivateKey),
		publicKeys:      make(map[string]*ecdsa.PublicKey),
		privateKeysLock: &sync.Mutex{},
		publicKeysLock:  &sync.Mutex{},
	}
}

func (d *defaultHandler) BuildRSAJWTToken(payload []byte, expireAt time.Time, keyID string) (token string, err error) {
	signingKey, ok := d.privateKeys[keyID]
	if !ok {
		return "", ErrKeyIdentifierNotFound
	}

	_token := jwt.NewWithClaims(jwt.SigningMethodES256, CustomClaim{
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: expireAt.Unix(),
			Issuer:    d.issuer,
		},
		Payload: base64.RawStdEncoding.EncodeToString(payload),
	})
	_token.Header[KID_HEADER] = keyID

	token, err = _token.SignedString(signingKey)
	if err != nil {
		return "", err
	}

	return token, nil
}

// https://pkg.go.dev/github.com/golang-jwt/jwt#example-Parse-Hmac
func (d *defaultHandler) ParseRSAJWTToken(token string, key string) (payload []byte, err error) {
	parsed, err := jwt.Parse(token, func(parsed *jwt.Token) (interface{}, error) {
		if _, ok := parsed.Method.(*jwt.SigningMethodECDSA); !ok {
			return nil, ErrInvalidSigningMethod //  token.Header["alg"]
		}

		// to make more secure, the endpoint also need to expect the key ID
		// key, ok := parsed.Header[KID_HEADER].(string)
		// if !ok {
		// 	return nil, err
		// }
		if key == "" {
			return nil, ErrKeyIdentifierEmpty
		}

		secret, ok := d.publicKeys[key]
		if !ok {
			return nil, ErrKeyIdentifierNotFound
		}

		return secret, nil
	})
	if err != nil {
		return nil, err
	}

	if !parsed.Valid {
		return nil, ErrInvalidToken
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidClaim
	}

	data, ok := claims[PAYLOAD_CLAIM]
	if !ok {
		return []byte(""), nil // just empty payload
	}

	b, ok := data.(string)
	if !ok {
		return nil, ErrInvalidPayload
	}

	result, err := base64.RawStdEncoding.DecodeString(b)
	if err != nil {
		return nil, ErrInvalidPayload
	}

	return result, nil
}

// StorePrivateKey stores private key, and also store the public key
func (d *defaultHandler) StorePrivateKey(keyID string, privateKeyPEM string) (err error) {
	d.privateKeysLock.Lock()
	defer d.privateKeysLock.Unlock()

	key, err := ParsePEMPrivateKey([]byte(privateKeyPEM))
	if err != nil {
		return err
	}
	d.privateKeys[keyID] = key

	// also store the public Key
	d.publicKeys[keyID] = &key.PublicKey

	return nil
}

func (d *defaultHandler) StorePublicKey(keyID string, publicKeyPEM string) (err error) {
	d.publicKeysLock.Lock()
	defer d.publicKeysLock.Unlock()

	key, err := ParsePEMPublicKey([]byte(publicKeyPEM))
	if err != nil {
		return err
	}

	d.publicKeys[keyID] = key
	return nil
}

func (d *defaultHandler) GetPublicKey(keyID string) (publicKey []byte, found bool, err error) {
	d.publicKeysLock.Lock()
	defer d.publicKeysLock.Unlock()

	pub, ok := d.publicKeys[keyID]
	if ok {
		value, err := EncodePEMPublicKey(pub)
		return value, true, err
	}

	priv, ok := d.privateKeys[keyID]
	if ok {
		value, err := EncodePEMPublicKey(&priv.PublicKey)
		return value, true, err
	}

	return nil, false, ErrKeyIdentifierNotFound
}

func ParsePEMPrivateKey(pem []byte) (*ecdsa.PrivateKey, error) {
	ecdsapk, err := jwt.ParseECPrivateKeyFromPEM(pem)
	return ecdsapk, err
}

func ParsePEMPublicKey(pem []byte) (*ecdsa.PublicKey, error) {
	ecdsapub, err := jwt.ParseECPublicKeyFromPEM(pem)
	return ecdsapub, err
}

func EncodePEMPublicKey(pub *ecdsa.PublicKey) (result []byte, err error) {
	b, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, err
	}

	result = pem.EncodeToMemory(&pem.Block{
		Type:    "ECDSA PUBLIC KEY",
		Headers: make(map[string]string),
		Bytes:   b,
	})

	return result, nil
}
