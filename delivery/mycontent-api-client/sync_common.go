package mycontentapiclient

import (
	"context"
	"net/url"
	"path"
	"sync"
	"time"

	"github.com/desain-gratis/common/delivery/mycontent-api/mycontent"
	content "github.com/desain-gratis/common/types/entity"
	"github.com/rs/zerolog/log"
)

type GetDependencies[T, U mycontent.Data] func(t T) []Context

type commonDep[T mycontent.Data] struct {
	syncer     *sync[T]
	client     *attachmentClient
	extract    Extract[T]
	uploadDir  string
	customPath func(mycontent.Data) string

	// TODO: moVE!!!
	authToken string
	namespace string

	// essential for refactor
	parentSyncer any
	childSyncer  any

	uploader           map[url.URL]Uploader[T]
	attachmentUploader map[url.URL]AttachmentUploader[T]

	dependencies           []mycontent.Data
	attachmentDependencies []mycontent.Data
}

type Uploader[T mycontent.Data] interface {
	Upload(ctx context.Context, data T) (T, error)
}

type syncG[T, U mycontent.Data] struct {
	client    *client[T]
	namespace string // filter namespace
	data      []T

	OptConfig OptionalConfig

	dependencies []*commonDep[T]
}

func (i *commonDep[T]) usync(ctx context.Context, data, parent mycontent.Data) (stat SyncStat, err error) {
	// if any(data).(mycontent.Attachment)

	// step 1
	// case 1: data only, parent nil. (the root entity)
	// case 2: data and parent.
	// case 3: data == nil / parent only --> invalid; return

	// step 2
	// apabila ada parent, harus ada fungsi yang bisa mapping balik / update balik parent (+ transformasi)
	// setelah data selesai di upload (atau bahkan belum upload parent sebelum data ada; khusus untuk data yng belum ada ID)
	// dah, coba itu dulu; tanpa logic attachment. Test.

	// step 3
	// sekaran ada tambahan logic attachment

}

type GGWP struct {
}

func (i *commonDep[T]) usyncData(ctx context.Context, key url.URL, data T) error {
	// get ID()
	if data.ID() == "" {
		synced, err := i.uploader[key].Upload(ctx, data) // simple client post inside
		if err != nil {
			log.Error().Msgf("Failed to update project definition %+v", err)
		}
		data.WithID(synced.ID())
		// maybe can sync all fields not only id later
	}

	wg := &sync.WaitGroup{}

	// get data dependency
	// for _, range :=
	// wg.Add(1)
	// go func() {
	// 	defer wg.Done()
	// }()

	// // get file dependency
	// go func() {
	// 	defer wg.Done()
	// }()

	wg.Wait()
}

// shared "sync" logic for both simple entity & entity with attachment
func (i *commonDep[T]) sync(ctx context.Context, dataArr []Context) (stat SyncStat, err error) {
	// 0. filter local entities inplace based on namespace
	// double guard... (usually in the i.Base already done its enough..)
	if i.syncer.namespace != "*" {
		var countValid int
		for idx := 0; idx < len(dataArr); idx++ {
			localEntity := dataArr[idx]

			if localEntity.Base.Namespace() != i.syncer.namespace {
				continue
			}

			dataArr[countValid] = localEntity
			countValid++
		}
		dataArr = dataArr[:countValid]
	}

	// 1. build local data map
	localData := map[string]Context{}
	for idx, pair := range dataArr {
		completeRefs := append(pair.Base.RefIDs(), pair.Base.ID())
		key := getKey(completeRefs, pair.Attachment.ID())
		localData[key] = dataArr[idx]
	}

	stat.LocalCount = len(localData)

	uploadDir := i.uploadDir

	localHash, errs := i.computeHashMulti(uploadDir, localData)
	if len(errs) > 0 {
		for _, err := range errs {
			log.Warn().Msgf("failed to compute hash %v", err)
		}
		log.Warn().Msgf(" images with error will be ignored. Please fix the error")
	}

	stat.LocalCountError = len(errs)

	_remoteAttachments, errUC := i.client.Get(context.Background(), i.authToken, i.syncer.namespace, nil, "") // "*" for all namespace
	if errUC != nil {
		log.Error().Msgf("%+v", errUC)
		return stat, errUC
	}
	remoteAttachments := i.attachmentToMap(_remoteAttachments)

	stat.RemoteCount = len(remoteAttachments)

	intersect := make(map[string]struct{})

	toOverwrite := make(map[string]Context)
	toDelete := make(map[string]mycontent.Data)

	for _, pair := range localData {
		completeRefs := append(pair.Base.RefIDs(), pair.Base.ID())
		key := getKey(completeRefs, pair.Attachment.ID())

		if _, ok := remoteAttachments[key]; ok {
			intersect[key] = struct{}{}
			stat.Intersect++
			continue
		}
		toOverwrite[key] = pair
		stat.ToAdd++
	}

	for _, remoteAttachment := range remoteAttachments {
		remoteID := getKey(remoteAttachment.RefIDs(), remoteAttachment.ID())
		if _, ok := localData[remoteID]; ok {
			continue
		}
		toDelete[remoteID] = remoteAttachment
		stat.ToDelete++
	}

	for key := range intersect {
		localData := localData[key]
		remoteAttachment := remoteAttachments[key]

		newdir := i.customDir(uploadDir, localData.Base)
		localHash, ok := localHash[completeUploadPath(newdir, localData.Attachment.URL())]
		if !ok {
			// likely not valid
			log.Debug().Msgf("May be a not valid data. Please check for WARNING/WRN messages in the log.")
			continue
		}

		hashable, ok := remoteAttachment.(Hashable)
		if ok {
			if localHash == hashable.Hash() {
				localData.Attachment = remoteAttachment
				stat.AlreadyInSync++
				continue
			}
		}

		// need re upload
		toOverwrite[key] = localData
		stat.ToSync++
	}

	// ====

	// Delete unused remote data
	for _, data := range toDelete {
		_, errUC := i.client.Delete(ctx, i.authToken, data.Namespace(), toRefsParam(i.client.refsParam, data.RefIDs()), data.ID())
		if errUC != nil {
			log.Error().Msgf("Failed to delete %v %v %v %v %v", i.client.endpoint, data.Namespace(), data.RefIDs(), data.ID(), errUC.Err())
			continue
		}
	}

	// Overwrite or create new remote data
	for _, localData := range toOverwrite {
		completeRefs := append(localData.Base.RefIDs(), localData.Base.ID())
		key := getKey(completeRefs, localData.Attachment.ID())

		//  switch Entity / File / Image specific logic here.
		// eg. if only simple entity, we just push the JSON
		// if it is an "Attachment" (we can type switch), then  we do upload logic, and update the parent base.
		// upload logic can have custom hook that translate the uploaded attachment back as "entity" type in parent entity
		locUploadDir := i.customDir(uploadDir, localData.Base)
		fileData, _, err := openStream(locUploadDir, localData.Attachment.URL())
		if err != nil {
			log.Error().Msgf("  failed to process image '%+v', msg: %+v", key, err)
			continue
		}

		payload := &content.Attachment{
			Id:           localData.Attachment.ID(),
			RefIds:       completeRefs,
			OwnerId:      localData.Base.Namespace(), // always the namespace of the base
			Name:         localData.Attachment.URL(),
			Hash:         localHash[completeUploadPath(locUploadDir, localData.Attachment.URL())],
			Description:  localData.Attachment.ID(),           // TODO: mycontent to add description
			Tags:         []string{localData.Attachment.ID()}, // TODO: mycontent to implement tags / ToAttachment
			ImageDataUrl: "",
			CreatedAt:    time.Now().Format(time.RFC3339),
		}
		// log.Info().Msgf("PAYLOD: %+v", payload)
		ra, err := i.client.Upload(ctx, i.authToken, payload.Namespace(), payload, "", fileData)
		if err != nil {
			log.Error().Msgf("  failed to sync an attachment in remote '%+v', msg: %+v", key, err)
			continue
		}

		// sync local data with remote
		localData.Attachment = attachmentToData(ra)
	}

	return stat, nil

}

func (i *commonDep[T]) customDir(dir string, base mycontent.Data) string {
	newdir := dir
	if i.customPath != nil {
		newdir = path.Join(dir, i.customPath(base))
	}
	return newdir
}

type Hashable interface {
	Hash() string
}

func (i *commonDep[T, U]) computeHashMulti(dir string, files map[string]Context) (map[string]string, []error) {

}
