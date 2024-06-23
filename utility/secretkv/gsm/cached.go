package gsm

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/desain-gratis/common/utility/secretkv"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/singleflight"
)

type cache struct {
	*gsmSecretProvider

	getCache      map[string]map[int]secretkv.Payload
	getCacheLock  *sync.Mutex
	listCache     map[string][]secretkv.Payload
	listCacheLock *sync.Mutex

	getCacheConfig  map[string]map[int]CacheConfig
	listCacheConfig map[string]CacheConfig

	group *singleflight.Group
}

type CacheConfig struct {
	PollDuration time.Duration
}

func NewCached(
	projectID int,
	getCacheConfig map[string]map[int]CacheConfig,
	listCacheConfig map[string]CacheConfig,
) *cache {
	cached := &cache{
		getCacheLock:    &sync.Mutex{},
		listCacheLock:   &sync.Mutex{},
		getCache:        make(map[string]map[int]secretkv.Payload),
		listCache:       make(map[string][]secretkv.Payload),
		getCacheConfig:  getCacheConfig,
		listCacheConfig: listCacheConfig,
		gsmSecretProvider: &gsmSecretProvider{
			projectID: projectID,
		},
		group: &singleflight.Group{},
	}

	for key, data := range getCacheConfig {
		for version, config := range data {
			go func(config CacheConfig, key string, version int) {
				initCtx := context.Background()
				initCtx = context.WithValue(initCtx, "name", "get-cache-config-bg")
				ticker := time.NewTicker(config.PollDuration)
				cached.Get(initCtx, key, version)
				for {
					select {
					case _ = <-ticker.C:
						log.Debug().Msgf("Poll key: %v ver: %v", key, version)
						cached.populateCacheGet(initCtx, key, version)
					}
				}
			}(config, key, version)
		}
	}

	for key, config := range listCacheConfig {
		go func(config CacheConfig, key string) {
			initCtx := context.Background()
			initCtx = context.WithValue(initCtx, "name", "list-cache-config-bg")
			ticker := time.NewTicker(config.PollDuration)
			cached.List(initCtx, key)
			for {
				select {
				case _ = <-ticker.C:
					cached.populateCacheList(initCtx, key)
				}
			}
		}(config, key)
	}

	return cached
}

func (c *cache) Get(ctx context.Context, key string, version int) (secretkv.Payload, error) {
	// check cache first
	if _, ok := c.getCache[key]; ok {
		if _, ok := c.getCache[key][version]; ok {
			log.Debug().Msg("Cache hit!")
			return c.getCache[key][version], nil
		}
	}

	return c.populateCacheGet(ctx, key, version)
}

func (c *cache) populateCacheGet(ctx context.Context, key string, version int) (secretkv.Payload, error) {
	var cacheConfigured bool
	if _, ok := c.getCacheConfig[key]; ok {
		if _, ok := c.getCacheConfig[key][version]; ok {
			cacheConfigured = true
		}
	}
	if !cacheConfigured {
		return c.gsmSecretProvider.Get(ctx, key, version)
	}

	sfKey := key + ":" + strconv.Itoa(version)
	sfResult := c.group.DoChan(sfKey, func() (any, error) {
		// Use context background to populate cache
		payload, err := c.gsmSecretProvider.Get(context.Background(), key, version)
		func() {
			c.getCacheLock.Lock()
			defer c.getCacheLock.Unlock()
			if _, ok := c.getCache[key]; !ok {
				c.getCache[key] = make(map[int]secretkv.Payload)
			}
			c.getCache[key][version] = payload
		}()
		return payload, err
	})

	select {
	case data := <-sfResult:
		// Result are received on time
		payload, _ := data.Val.(secretkv.Payload)
		if data.Shared {
			log.Debug().Msgf("Singleflight working for key %v!", sfKey)
		}
		return payload, data.Err
	case _ = <-ctx.Done():
		// Client get timeout first
		return secretkv.Payload{}, errors.New("timeout")
	}
}

func (c *cache) List(ctx context.Context, key string) ([]secretkv.Payload, error) {
	// check cache first
	if _, ok := c.listCache[key]; ok {
		log.Debug().Msg("Cache hit!")
		return c.listCache[key], nil
	}

	return c.populateCacheList(ctx, key)
}

func (c *cache) populateCacheList(ctx context.Context, key string) ([]secretkv.Payload, error) {
	var cacheConfigured bool

	if _, ok := c.listCacheConfig[key]; ok {
		cacheConfigured = true
	}

	if !cacheConfigured {
		return c.gsmSecretProvider.List(ctx, key)
	}

	sfKey := key
	sfResult := c.group.DoChan(sfKey, func() (any, error) {
		// Use context background to populate cache
		log.Debug().Msgf("We're processing %v, please be patient (%v)", sfKey, ctx.Value("name"))
		payload, err := c.gsmSecretProvider.List(context.Background(), key)
		func() {
			c.listCacheLock.Lock()
			defer c.listCacheLock.Unlock()
			c.listCache[key] = payload
		}()
		log.Debug().Msgf("Were done processing %v (%v)", sfKey, ctx.Value("name"))
		return payload, err
	})

	select {
	case data := <-sfResult:
		// Result are received on time
		payload, _ := data.Val.([]secretkv.Payload)
		if data.Shared {
			log.Debug().Msgf("Singleflight List working for key %v! (%v)", sfKey, ctx.Value("name"))
		}
		return payload, data.Err
	case _ = <-ctx.Done():
		// Client get timeout first
		return []secretkv.Payload{}, errors.New("timeout")
	}
}
