package main

import (
	"context"
	"encoding/json"
	"os"
	"strconv"

	"github.com/rs/zerolog/log"
)

type Config map[string]any

var CONFIG Config = make(map[string]any)

func (c Config) GetString(key string) string {
	value, ok := c[key]
	if !ok {
		log.Fatal().Msgf("Key not found: '%v'", key)
		return ""
	}

	strvalue, ok := value.(string)
	if !ok {
		log.Fatal().Msgf("Failed to assert config with key '%v' as string", key)
		return ""
	}

	return strvalue
}

func (c Config) GetInt(key string) int {
	var err error

	value, ok := c[key]
	if !ok {
		log.Fatal().Msgf("Key not found: '%v'", key)
		return 0
	}

	strvalue, ok := value.(string)
	if !ok {
		log.Fatal().Msgf("Failed to assert config with key '%v' as string", key)
		return 0
	}

	result, err := strconv.Atoi(strvalue)
	if err != nil {
		log.Fatal().Msgf("Failed to parse value '%v' as int for config key '%v'", strvalue, key)
		return 0
	}

	return result
}

func (c Config) GetFloat64(key string) float64 {
	var err error

	value, ok := c[key]
	if !ok {
		log.Fatal().Msgf("Key not found: '%v'", key)
		return 0
	}

	strvalue, ok := value.(string)
	if !ok {
		log.Fatal().Msgf("Failed to assert config with key '%v' as string", key)
		return 0
	}

	result, err := strconv.ParseFloat(strvalue, 64)
	if err != nil {
		log.Fatal().Msgf("Failed to parse value '%v' as float64 for config key '%v'", strvalue, key)
		return 0
	}

	return result
}

func initConfig(ctx context.Context, path, env string) (err error) {
	cfgcombined := make(map[string]any)

	filePath := path + env + ".json"

	log.Info().Msgf("Reading config from: %v", filePath)
	// Read config
	cfg, err := os.ReadFile(filePath)
	if err != nil {
		log.Warn().Msgf("Failed to read config: %v", err)
	} else {
		// Load config
		err = json.Unmarshal(cfg, &cfgcombined)
		if err != nil {
			return err
		}
		CONFIG = cfgcombined
	}

	// overwrite config with secret
	secret := `
		{"example.secret.format": "kv"}
	`

	// obtain from your external secret provider..
	_ = ctx

	// combine
	err = json.Unmarshal([]byte(secret), &cfgcombined)
	if err != nil {
		log.Error().Msgf("  Failed to parse secret: %v", err)
		return err
	}

	CONFIG = cfgcombined

	return nil
}
