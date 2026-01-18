package mycontentapiclient

import (
	"context"
	"net/http"
	"path"
	"time"

	content "github.com/desain-gratis/common/types/entity"
	types "github.com/desain-gratis/common/types/http"
	"github.com/rs/zerolog/log"
)

func (i *fileDep[T]) syncFiles(dataArr []FileContext[T]) (stat SyncStat, errUC *types.CommonError) {
	ctx := context.Background()

	// 0. filter local entities inplace based on namespace
	// double guard... (usually in the i.Base already done its enough..)
	if i.sync.namespace != "*" {
		var countValid int
		for idx := 0; idx < len(dataArr); idx++ {
			localEntity := dataArr[idx]

			if localEntity.Base.Namespace() != i.sync.namespace {
				continue
			}

			dataArr[countValid] = localEntity
			countValid++
		}
		dataArr = dataArr[:countValid]
	}

	// 1. build local data map
	localData := map[string]FileContext[T]{}
	for idx, pair := range dataArr {
		completeRefs := append(pair.Base.RefIDs(), pair.Base.ID())
		key := getKey(completeRefs, (*pair.File).Id)
		localData[key] = dataArr[idx]
	}

	stat.LocalCount = len(localData)

	uploadDir := i.uploadDir

	localHash, errUCs1 := i.computeFileConfigHashMulti(uploadDir, localData)
	if len(errUCs1) > 0 {
		for _, errUC := range errUCs1 {
			log.Warn().Msgf("\n%v %+v", errUC.Code, errUC.Message)
		}
		log.Warn().Msgf(" images with error will be ignored. Please fix the error")
	}

	stat.LocalCountError = len(errUCs1)

	_remoteAttachments, errUC := i.client.Get(context.Background(), i.sync.OptConfig.AuthorizationToken, i.sync.namespace, nil, "") // "*" for all namespace
	if errUC != nil {
		log.Error().Msgf("%+v", errUC)
		return stat, errUC
	}
	remoteAttachments := attachmentToMap(_remoteAttachments)

	stat.RemoteCount = len(remoteAttachments)

	intersect := make(map[string]struct{})

	toOverwrite := make(map[string]FileContext[T])
	toDelete := make(map[string]*content.Attachment)

	for _, pair := range localData {
		completeRefs := append(pair.Base.RefIDs(), pair.Base.ID())
		key := getKey(completeRefs, (*pair.File).Id)

		if _, ok := remoteAttachments[key]; ok {
			intersect[key] = struct{}{}
			stat.Intersect++
			continue
		}
		toOverwrite[key] = pair
		stat.ToAdd++
	}

	for _, remoteAttachment := range remoteAttachments {
		remoteID := getKey((*remoteAttachment).RefIds, (*remoteAttachment).Id)
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
		localHash, ok := localHash[completeUploadPath(newdir, (**localData.File).Url)]
		if !ok {
			// likely not valid
			log.Debug().Msgf("May be a not valid data. Please check for WARNING/WRN messages in the log.")
			continue
		}

		remoteHash := remoteAttachment.Hash
		if localHash == remoteHash {
			// sync local data with remote
			*localData.File = attachmentToFile(remoteAttachment)
			stat.AlreadyInSync++
			continue
		}

		// need re upload
		toOverwrite[key] = localData
		stat.ToSync++
	}

	// ====

	// Delete unused remote data
	for _, data := range toDelete {
		_, errUC := i.client.Delete(ctx, i.sync.OptConfig.AuthorizationToken, data.Namespace(), toRefsParam(i.client.refsParam, data.RefIds), data.Id)
		if errUC != nil {
			log.Error().Msgf("Failed to delete %v %v %v %v %v", i.client.endpoint, data.Namespace(), data.RefIDs(), data.ID(), errUC.Err())
			continue
		}
	}

	// Overwrite or create new remote data
	for _, localData := range toOverwrite {
		completeRefs := append(localData.Base.RefIDs(), localData.Base.ID())
		key := getKey(completeRefs, (*localData.File).Id)
		locUploadDir := i.customDir(uploadDir, localData.Base)
		fileData, _, errUC := processFile(locUploadDir, localData.File)
		if errUC != nil {
			log.Error().Msgf("  failed to process file '%+v', msg: %+v", key, errUC)
			continue
		}

		payload := &content.Attachment{
			Id:           (*localData.File).Id,
			RefIds:       completeRefs,
			OwnerId:      localData.Base.Namespace(), // always the namespace of the base
			Name:         (*localData.File).Url,
			Hash:         localHash[completeUploadPath(locUploadDir, (**localData.File).Url)],
			Description:  (*localData.File).Description,
			Tags:         (*localData.File).Tags,
			ContentSize:  uint64(len(fileData)),
			ImageDataUrl: "",
			CreatedAt:    time.Now().Format(time.RFC3339),
		}
		// log.Info().Msgf("PAYLOD: %+v", payload)
		ra, err := i.client.Upload(ctx, i.sync.OptConfig.AuthorizationToken, payload.Namespace(), payload, "", fileData)
		if err != nil {
			log.Error().Msgf("  failed to sync an attachment in remote '%+v', msg: %+v", key, err)
			continue
		}

		// sync local data with remote
		*localData.File = attachmentToFile(ra)
	}

	return stat, nil

}

func (i *fileDep[T]) computeFileConfigHashMulti(dir string, files map[string]FileContext[T]) (map[string]string, []*types.Error) {
	id2hash := make(map[string]string)
	errUC := make([]*types.Error, 0)
	for _, file := range files {
		newdir := i.customDir(dir, file.Base)
		imgHash, err := computeFileConfigHash(newdir, *file.File)
		if err != nil {
			errUC = append(errUC, &types.Error{
				HTTPCode: http.StatusBadRequest, Code: "CLIENT_ERROR", Message: "Cannot open file '" + (*file.File).Url + "' or compute its hash.\n Make sure you entered a valid image at that path.\n error: " + err.Error(),
			})
			continue
		}
		id2hash[completeUploadPath(newdir, (**file.File).Url)] = imgHash
	}

	return id2hash, errUC
}

func (i *fileDep[T]) customDir(dir string, base T) string {
	newdir := dir
	if i.customPath != nil {
		newdir = path.Join(dir, i.customPath(base))
	}
	return newdir
}
