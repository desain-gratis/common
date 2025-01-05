package jwt

import (
	"io"
	"os"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsimple"
	"github.com/rs/zerolog/log"

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
