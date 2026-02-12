package mycontentapiclient

import (
	"context"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/desain-gratis/common/delivery/mycontent-api/mycontent"
	"github.com/desain-gratis/common/types/entity"
	content "github.com/desain-gratis/common/types/entity"
)

type ImageContext[T mycontent.Data] struct {
	Base  T
	Image **entity.Image
}

type FileContext[T mycontent.Data] struct {
	Base T
	File **entity.File
}

type ExtractImages[T mycontent.Data] func(t []T) []ImageContext[T]
type ExtractFiles[T mycontent.Data] func(t []T) []FileContext[T]
type ExtractOtherEntities[T any] func(t []T) []mycontent.Data

type fileDep[T mycontent.Data] struct {
	sync       *sync[T]
	client     *attachmentClient
	extract    ExtractFiles[T]
	uploadDir  string
	customPath func(T) string
}

type sync[T mycontent.Data] struct {
	client    *client[T]
	namespace string // filter namespace
	data      []T

	OptConfig OptionalConfig

	imageDeps []*imageDep[T]
	fileDeps  []*fileDep[T]
}

type OptionalConfig struct {
	AuthorizationToken string

	// Perform a hard sync (make serverthe same as client, by deleting content in the server)
	Hard bool
}

func Sync[T mycontent.Data](client *client[T], namespace string, data []T, optConfig OptionalConfig) *sync[T] {
	if namespace == "" {
		log.Fatal().Msgf("please provide namespace explicitly. Put '*' to sync all")
	}

	return &sync[T]{
		client:    client,
		namespace: namespace,
		data:      data,
		OptConfig: optConfig,
	}
}

func (s *sync[T]) WithImages(
	client *attachmentClient,
	extract ExtractImages[T],
	uploadDir string,
	customPath func(t T) string,
) *sync[T] {
	s.imageDeps = append(s.imageDeps, &imageDep[T]{
		sync:       s,
		client:     client,
		extract:    extract,
		uploadDir:  uploadDir,
		customPath: customPath,
	})

	return s
}

func (s *sync[T]) WithFiles(client *attachmentClient, extract ExtractFiles[T], uploadDir string) *sync[T] {
	s.fileDeps = append(s.fileDeps, &fileDep[T]{
		sync:      s,
		client:    client,
		extract:   extract,
		uploadDir: uploadDir,
	})
	return s
}

func (s *sync[T]) Execute(ctx context.Context) error {
	// 1. get all main entity from remote, for all namespace
	remoteEntities, err := s.client.Get(ctx, s.namespace, nil, "") // "*" special namespace to get all namespace
	remoteEntitiesMap := make(map[string]T)
	if err != nil {
		log.Error().Msgf("%+v", err)
		return err
	}
	for _, remoteEntity := range remoteEntities {
		remoteID := getKey2(remoteEntity)
		remoteEntitiesMap[remoteID] = remoteEntity
	}

	// 2. get main entity in local
	localEntities := s.data

	// 2a. filter local entities inplace based on namespace
	if s.namespace != "*" {
		var countValid int
		for idx := 0; idx < len(localEntities); idx++ {
			localEntity := localEntities[idx]

			if localEntity.Namespace() != s.namespace {
				continue
			}

			localEntities[countValid] = localEntity
			countValid++
		}
		localEntities = localEntities[:countValid]
	}

	// 3. check if local project exist in server, if not create one, only for entity that doesn't have ID yet.
	// ID is used if the local entity have file or image dependencies.

	// TODO: using goroutine pool
	for _, localEntity := range localEntities {
		key := getKey2(localEntity)
		if _, ok := remoteEntitiesMap[key]; !ok && localEntity.ID() == "" {
			synced, err := s.client.Post(ctx, localEntity, nil)
			if err != nil {
				log.Error().Msgf("Failed to create entity of type %T with key %v: %v", localEntity, key, err)
				continue
			}
			localEntity.WithID(synced.ID())
		}
	}

	localEntitiesMap := make(map[string]T)
	for _, localEntity := range localEntities {
		key := getKey2(localEntity)
		localEntitiesMap[key] = localEntity
	}

	// 4. inversely, for all remote project that is not in local, delete them
	for _, remoteEntity := range remoteEntities {
		if !s.OptConfig.Hard {
			break
		}
		remoteID := getKey2(remoteEntity)
		if _, ok := localEntitiesMap[remoteID]; !ok {
			_, err := s.client.Delete(ctx, remoteEntity.Namespace(), remoteEntity.RefIDs(), remoteEntity.ID())
			if err != nil {
				log.Error().Msgf("Failed to delete remote entity with id: %v err: %v", remoteID, err)
			}
		}
	}

	// 5. Now the entity are synced. But the dependencies are not yet.
	//    Later, we need to update the entity again based on the result of the dependency.

	// 5. Collect dependency information in the project
	//    Prepare for their respective endpoint

	if len(s.imageDeps) > 0 {
		log.Info().Msgf("	Syncing images dependencies..")
	}

	for _, imgDep := range s.imageDeps {
		imageRefs := imgDep.extract(localEntities) // pointer to images, for inplace update
		// todo: implement worker thread
		stat, err := imgDep.syncImages(imageRefs)
		if err != nil {
			log.Error().Msgf("	Failed to sync project thumbnails. Err: %v", err)
			continue
		}
		log.Info().Msgf(`Sync project's thumbnails statistics:
		%+v`, stat)
	}

	// TODO: sync file dependencies as well
	if len(s.fileDeps) > 0 {
		log.Info().Msgf("	Syncing file dependencies..")
	}
	for _, fileDep := range s.fileDeps {
		fileRefs := fileDep.extract(localEntities) // pointer to images, for inplace update
		// todo: implement worker thread
		stat, err := fileDep.syncFiles(fileRefs)
		if err != nil {
			log.Error().Msgf("	Failed to sync project file. Err: %v", err)
			continue
		}
		log.Info().Msgf(`Sync project's file statistics:
		%+v`, stat)
	}

	// Sync back the project since the data in localProjects have been already modified
	for _, localEntity := range localEntities {
		// TODO: calculate hash or compare directly to optimize upload
		_, err = s.client.Post(ctx, localEntity, nil)
		if err != nil {
			log.Error().Msgf("Failed to update project definition %+v", err)
		}
	}

	return nil
}

func attachmentToThumbnails(input map[string]*content.Attachment) map[string]*entity.Image {
	result := make(map[string]*entity.Image)
	for k, v := range input {
		result[k] = &entity.Image{
			Id:           v.Id,
			ThumbnailUrl: v.Url, // not exist
			DataUrl:      v.ImageDataUrl,
			Tags:         v.Tags,
			Description:  v.Description,
			Url:          v.Url,
		}
	}
	return result
}

// for checking differences within connection
func getKey(refIDs []string, ID string) string {
	var result string
	if len(refIDs) > 0 {
		result = strings.Join(refIDs, "|") + "|"
	}
	result += ID
	return result
}

func getKey2(content mycontent.Data) string {
	return getKey(content.RefIDs(), content.ID())
}
