package mycontentapiclient

import (
	"context"
	"strings"

	"github.com/desain-gratis/common/types/entity"
	content "github.com/desain-gratis/common/types/entity"
	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/usecase/mycontent"
	"github.com/rs/zerolog/log"
)

type ImageContextPair[T mycontent.Data] struct {
	Base  T
	Image **entity.Image
}

type ExtractImages[T mycontent.Data] func(t []T) []ImageContextPair[T]
type ExtractFiles[T mycontent.Data] func(t []T) []**entity.File
type ExtractOtherEntities[T any] func(t []T) []mycontent.Data

type fileDep[T mycontent.Data] struct {
	client          *client[*entity.Attachment]
	extract         ExtractFiles[T]
	uploadDirectory string
	namespace       string
}

type sync[T mycontent.Data] struct {
	client    *client[T]
	imageDeps []imageDep[T]
	fileDeps  []fileDep[T]
	data      []T
	namespace string
}

func Sync[T mycontent.Data](client *client[T], data []T, namespace string) *sync[T] {
	return &sync[T]{
		client:    client,
		data:      data,
		namespace: namespace,
	}
}

func (s *sync[T]) WithImages(client *attachmentClient, extract ExtractImages[T], uploadDirectory string) *sync[T] {
	s.imageDeps = append(s.imageDeps, imageDep[T]{
		client:          client,
		extract:         extract,
		uploadDirectory: uploadDirectory,
		namespace:       s.namespace,
	})

	return s
}

func (s *sync[T]) WithFiles(client *client[*entity.Attachment], extract ExtractFiles[T], uploadDirectory string) *sync[T] {
	s.fileDeps = append(s.fileDeps, fileDep[T]{
		client:          client,
		extract:         extract,
		uploadDirectory: uploadDirectory,
		namespace:       s.namespace,
	})
	return s
}

func (s *sync[T]) Execute(ctx context.Context) *types.CommonError {

	// 1. get all main entity from remote
	remoteEntities, errUC := s.client.Get(ctx, s.namespace, nil, "")
	remoteEntitiesMap := make(map[string]T)
	if errUC != nil {
		log.Error().Msgf("%+v", errUC)
		return errUC
	}
	for _, remoteEntity := range remoteEntities {
		remoteID := getID(remoteEntity.RefIDs(), remoteEntity.ID())
		remoteEntitiesMap[remoteID] = remoteEntity
	}

	// 2. get main entity in local
	localEntities := s.data
	localEntitiesMap := make(map[string]T)
	for _, localEntity := range localEntities {
		id := getID(localEntity.RefIDs(), localEntity.ID())
		localEntitiesMap[id] = localEntity
	}

	// 3. check if local project exist in server, if not create one
	// TODO: using goroutine pool
	for _, localEntity := range localEntities {
		id := getID(localEntity.RefIDs(), localEntity.ID())
		if _, ok := localEntitiesMap[id]; !ok {
			_, errUC := s.client.Post(ctx, localEntity)
			if errUC != nil {
				log.Error().Msgf("Failed to create entity of type %T with ID %v", localEntity, id)
			}
		}
	}

	// 4. inversely, for all remote project that is not in local, delete them
	for _, remoteEntity := range remoteEntities {
		remoteID := getID(remoteEntity.RefIDs(), remoteEntity.ID())
		if _, ok := localEntitiesMap[remoteID]; !ok {
			// s.namespace instead of not remoteEntity.Namespace()
			_, errUC := s.client.Delete(ctx, s.namespace, convertRefsParam(s.client.refsParam, remoteEntity.RefIDs()), remoteEntity.ID())
			if errUC != nil {
				log.Error().Msgf("Failed to delete project Id %v", remoteID)
			}
		}
	}

	// 5. Now the entity are synced. But the dependencies are not yet.
	//    Later, we need to update the entity again based on the result of the dependency.

	// 5. Collect dependency information in the project
	//    Prepare for their respective endpoint

	log.Info().Msgf("	Syncing images dependencies..")
	for _, imgDep := range s.imageDeps {
		imageRefs := imgDep.extract(localEntities) // pointer to images, for inplace update
		stat, err := imgDep.syncImages(imageRefs)
		if errUC != nil {
			log.Error().Msgf("	Failed to sync project thumbnails. Err: %v", err)
			continue
		}
		log.Info().Msgf(`Sync project's thumbnails statistics:
		%+v`, stat)
	}

	// Sync back the project since the data in localProjects have been already modified
	for _, localEntity := range localEntities {
		// TODO: calculate hash or compare directly to optimize upload

		_, errUC = s.client.Post(ctx, localEntity)
		if errUC != nil {
			log.Error().Msgf("Failed to update project definition %+v", errUC)
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

func convertRefsParam(refsParam []string, refIDs []string) map[string]string {
	if len(refsParam) != len(refIDs) {
		log.Fatal().Msgf("Parameter not matching!")
	}
	result := make(map[string]string, len(refsParam))
	for i := range refIDs {
		result[refsParam[i]] = refIDs[i]
	}

	return result
}

// for checking differences within connection
func getID(refIDs []string, ID string) string {
	result := ID
	if len(refIDs) > 0 {
		result = strings.Join(refIDs, "|") + "|" + ID
	}
	return result
}
