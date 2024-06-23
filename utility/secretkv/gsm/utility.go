package gsm

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/rs/zerolog/log"
	"google.golang.org/api/iterator"

	"github.com/desain-gratis/common/utility/secretkv"
)

var client *secretmanager.Client
var err error
var initCtx <-chan struct{}

// TODO no init at here, since we're packaging solution using "Delivery",
// if our application does not need gsm authentication we want to avoid init
func init() {
	_initCtx := make(chan struct{})
	initCtx = _initCtx

	// Create connection asynchronously so it will not block initialization
	go func() {
		defer close(_initCtx)

		// TODO exponential retry until success
		client, err = secretmanager.NewRESTClient(context.Background())
		if err != nil {
			log.Err(err).Msgf(`Unable to create Google Secret Manager (GSM) client.
	Make sure you to configure your local development environment.
	Install gcloud cli and configure them properly.
	TODO: create the docs.
	`)
		}
	}()
}

func checkInit(requestCtx context.Context) (*secretmanager.Client, error) {
	select {
	case <-initCtx:
		return client, err
	case <-requestCtx.Done():
		return nil, errors.New("timeout")
	}
}

var _ secretkv.Provider = &gsmSecretProvider{}

type gsmSecretProvider struct {
	projectID int
}

func New(projectID int) *gsmSecretProvider {
	return &gsmSecretProvider{
		projectID: projectID,
	}
}

func (g *gsmSecretProvider) Get(ctx context.Context, key string, version int) (secretkv.Payload, error) {
	client, err := checkInit(ctx)
	if err != nil {
		// too long at init, exceeding timeout of the ctx, in this case, we communicate to user,
		// any functionality using this will not be available
		//
		// or, after trying to retry multiple times but failed
		return secretkv.Payload{}, err
	}
	versionText := "latest"
	if version > 0 {
		versionText = strconv.Itoa(version)
	}

	sec, err := client.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{
		Name: "projects/" + strconv.Itoa(g.projectID) + "/secrets/" + key + "/versions/" + versionText,
	})
	if err != nil {
		return secretkv.Payload{}, err
	}

	key, version, err = getKeyAndVersion(sec.Name)
	if err != nil {
		return secretkv.Payload{}, err
	}

	return secretkv.Payload{
		Key:     key,
		Version: version,
		Payload: sec.GetPayload().GetData(),
		Meta: map[string]any{
			"sync_time": time.Now(),
		},
	}, nil
}

func (g *gsmSecretProvider) List(ctx context.Context, key string) ([]secretkv.Payload, error) {
	client, err := checkInit(ctx)
	if err != nil {
		// too long at init, exceeding timeout of the ctx, in this case, we communicate to user,
		// any functionality using this will not be available
		//
		// or, after trying to retry multiple times but failed
		return nil, err
	}

	iter := client.ListSecretVersions(ctx, &secretmanagerpb.ListSecretVersionsRequest{
		Parent: "projects/" + strconv.Itoa(g.projectID) + "/secrets/" + key,
	})

	var versions []secretkv.Payload
	for {
		data, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Err(err).Msgf("Failed to get data for secret in %v name: %v", g.projectID, key)
			return versions, err
		}

		// Filter out non enabled keys
		if data.State != secretmanagerpb.SecretVersion_ENABLED {
			continue
		}

		key, version, err := getKeyAndVersion(data.Name)
		if err != nil {
			log.Warn().Msgf("Failed to parse GSM secret in %v name: %v version: %v", g.projectID, key, version)
			continue
		}

		payload, err := g.Get(ctx, key, version)
		if err != nil {
			log.Warn().Msgf("Failed to get individiual secret in %v name: %v version: %v err: %v", g.projectID, key, version, err)
			continue
		}

		payload.Meta = map[string]any{
			"sync_time": time.Now(),
		}

		payload.CreatedAt = data.CreateTime.AsTime()

		versions = append(versions, payload)
	}

	return versions, err
}

func (g *gsmSecretProvider) GetF(key string, version int) func() (secretkv.Payload, error) {
	return func() (secretkv.Payload, error) {
		return g.Get(context.Background(), key, version)
	}
}

func getKeyAndVersion(fullName string) (string, int, error) {
	token := strings.Split(fullName, "/")
	if len(token) < 3 {
		return "", 0, errors.New("Error parsing secret name")
	}
	version, err := strconv.Atoi(token[len(token)-1])
	if err != nil {
		return "", 0, err
	}

	key := token[len(token)-3]

	return key, version, nil
}
