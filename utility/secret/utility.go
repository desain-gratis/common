package jwt

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsimple"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/encoding/protojson"

	authapi "github.com/desain-gratis/common/types/protobuf/service/auth-api"
	secret_hmac "github.com/desain-gratis/common/utility/secret/hmac"
	secret_kv "github.com/desain-gratis/common/utility/secret/kv"
	secret_rsa "github.com/desain-gratis/common/utility/secret/rsa"
)

type Secret struct {
	KV    map[string]string `hcl:"kv"`
	ECDSA map[string]string `hcl:"ecdsa"`
	HMAC  map[string]string `hcl:"hmac"`
}

type Strategy string

const (
	STRATEGY_HCL_STDIN Strategy = "hcl-stdin"
	STRATEGY_GSM       Strategy = "gsm"
	STRATEGY_FLAG      Strategy = "hcl-flag"
)

func Load(secretPath string, implementation Strategy) {
	switch implementation {
	case STRATEGY_FLAG:
		f, err := os.Open(secretPath)
		if err != nil {
			log.Err(err).Msgf("Cannot load secret")
		}
		loadHCLSecretFromStdin(f)
	case STRATEGY_HCL_STDIN:
		loadHCLSecretFromStdin(os.Stdin)
	case STRATEGY_GSM:
		log.Error().Str("init", "SECRET STRATEGY GSM IS NOT IMPLEMENTED")
		// no op (currently not supported)
	default:
	}
}

func LoadSecretFromServiceAPIHTTP(accountAPIAddress string) error {
	go func() {
		clientz := http.Client{
			Timeout: 100 * time.Millisecond,
		}

		// conn, err := grpc.Dial(accountAPIAddress, opts...)
		u, err := url.Parse(accountAPIAddress)
		if err != nil {
			log.Err(err).Msgf("GEGE")
			return
		}

	HHEE:
		resp, err := clientz.Do(&http.Request{
			Method: http.MethodGet,
			URL:    u,
		})
		if err != nil {
			log.Err(err).Str("address", accountAPIAddress).Msgf("Failed to gte signing key")
			return
		}

		glug, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Err(err).Str("address", accountAPIAddress).Msgf("Failed to gte signing key")
			return
		}

		var signingKeyResp *authapi.SigningKeyResponse = &authapi.SigningKeyResponse{}
		err = protojson.UnmarshalOptions{
			DiscardUnknown: true,
		}.Unmarshal(glug, signingKeyResp)
		if err != nil {
			log.Err(err).Str("address", accountAPIAddress).Msgf("Failed to unmarshal signing key")
			return
		}

		if err == nil {
			if signingKeyResp.Error == nil {
				secret_rsa.StorePublicKey(signingKeyResp.Success.KeyId, signingKeyResp.Success.KeyPem)
				log.Info().Str("address", accountAPIAddress).Msgf("Stored the account API public key. See you in the next 1 hour")
			} else {
				log.Err(err).Str("address", accountAPIAddress).Msgf(signingKeyResp.Error.Message)
			}

		} else {
			log.Err(err).Str("address", accountAPIAddress).Msgf("Failed to gte signing key")
		}

		time.Sleep(1 * time.Hour)
		goto HHEE

	}()
	return nil
}

// many more loader later

func loadHCLSecretFromStdin(in io.Reader) error {
	reader := io.LimitReader(in, 10*1<<20)
	bytes, err := io.ReadAll(reader)
	if err != nil {
		log.Error().AnErr("init", err)
		return err
	}

	// 2. Read config from flag
	var cfg Secret
	err = hclsimple.Decode("secret.hcl", bytes, nil, &cfg)
	if err != nil {
		diag := err.(hcl.Diagnostics)
		log.Error().Errs("init", diag.Errs())
		log.Fatal().Msgf("Failed to open secret.\nError: '%v'", err)
		return err
	}

	for k, v := range cfg.ECDSA {
		err := secret_rsa.StorePrivateKey(k, v)
		if err != nil {
			log.Err(err).Msgf("Failed storing ECSDA secret `%v`", k)
		} else {
			log.Info().Msgf("Stored  ECSDA secret `%v`", k)
		}
		// v, found, err := jwtrsa.GetPublicKey(k)
		// log.Println("SON", string(v), found, err)
	}
	for k, v := range cfg.HMAC {
		err := secret_hmac.Store(k, v)
		if err != nil {
			log.Err(err).Msgf("Failed storing HMAC secret `%v`", k)
		} else {
			log.Info().Msgf("Stored  HMAC secret `%v`", k)
		}
	}
	for k, v := range cfg.KV {
		err := secret_kv.Store(k, v)
		if err != nil {
			log.Err(err).Msgf("Failed storing KV secret `%v`", k)
		} else {
			log.Info().Msgf("Stored KV secret `%v`", k)
		}
	}

	return nil
}
