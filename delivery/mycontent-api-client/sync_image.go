package mycontentapiclient

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"hash/fnv"
	"image"
	"io"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/desain-gratis/common/delivery/mycontent-api-client/imageproc"
	"github.com/desain-gratis/common/types/entity"
	content "github.com/desain-gratis/common/types/entity"
	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/usecase/mycontent"
	"github.com/disintegration/imaging"
	"github.com/kolesa-team/go-webp/webp"
	"github.com/rs/zerolog/log"
	"golang.org/x/image/draw"
)

type SyncStat struct {
	LocalCount      int
	LocalCountError int
	RemoteCount     int
	Intersect       int
	ToSync          int
	AlreadyInSync   int
	ToAdd           int
	ToDelete        int
}

type imageDep[T mycontent.Data] struct {
	namespace       string
	client          *attachmentClient
	extract         ExtractImages[T]
	uploadDirectory string
}

func (i *imageDep[T]) syncImages(dataArr []ImageContextPair[T]) (stat SyncStat, errUC *types.CommonError) {
	ctx := context.Background()

	data := map[string]ImageContextPair[T]{}
	for idx, pair := range dataArr {
		completeRefs := pair.Base.RefIDs()
		completeRefs = append(completeRefs, pair.Base.ID())
		datum := pair.Image
		id := getID(completeRefs, (*datum).Id)
		data[id] = dataArr[idx]
	}

	stat.LocalCount = len(data)

	localHash, errUCs1 := i.computeImageConfigHashMulti(i.uploadDirectory, data)
	if len(errUCs1) > 0 {
		for _, errUC := range errUCs1 {
			log.Warn().Msgf("\n%v %+v", errUC.Code, errUC.Message)
		}
		log.Warn().Msgf(" images with error will be ignored.")
	}

	stat.LocalCountError = len(data) - len(localHash)

	_remoteAttachments, errUC := i.client.Get(context.Background(), i.namespace, nil, "")
	if errUC != nil {
		log.Error().Msgf("%+v", errUC)
		return stat, errUC
	}
	remoteAttachments := attachmentToMap(_remoteAttachments)

	stat.RemoteCount = len(remoteAttachments)

	intersect := make(map[string]struct{})

	toOverwrite := make(map[string]ImageContextPair[T])
	toDelete := make(map[string]*content.Attachment)

	for _, pair := range data {
		completeRefs := pair.Base.RefIDs()
		completeRefs = append(completeRefs, pair.Base.ID())
		datum := pair.Image
		id := getID(completeRefs, (*datum).Id)

		if _, ok := remoteAttachments[id]; ok {
			intersect[id] = struct{}{}
			stat.Intersect++
			continue
		}
		toOverwrite[id] = pair
		stat.ToAdd++
	}

	for _, remoteAttachment := range remoteAttachments {
		remoteID := getID((*remoteAttachment).RefIds, (*remoteAttachment).Id)
		if _, ok := data[remoteID]; ok {
			continue
		}
		toDelete[remoteAttachment.Id] = remoteAttachment
		stat.ToDelete++
	}

	for id := range intersect {
		localData := data[id]
		remoteAttachment := remoteAttachments[id]

		localHash, ok := localHash[i.uploadDirectory+"|"+(*localData.Image).Url]
		if !ok {
			// likely not valid
			log.Debug().Msgf("May be a not valid data. Please check for WARNING/WRN messages in the log.")
			continue
		}

		remoteHash := remoteAttachment.Hash
		if localHash == remoteHash {
			// sync local data with remote
			*localData.Image = attachmentToImage(remoteAttachment)
			stat.AlreadyInSync++
			continue
		}

		// need re upload
		toOverwrite[id] = localData
		stat.ToSync++
	}

	// ====

	// Delete unused remote data
	for _, data := range toDelete {
		_, errUC := i.client.Delete(ctx, i.namespace, convertRefsParam(i.client.refsParam, data.RefIds), data.Id)
		if errUC != nil {
			log.Error().Msgf("Failed to delete %v %v %v %v %v", i.client.endpoint, i.namespace, data.RefIds, data.Id, errUC.Err())
			continue
		}
	}

	// Overwrite or create new remote data
	for _, localData := range toOverwrite {
		dir := i.uploadDirectory

		completeRefs := localData.Base.RefIDs()
		completeRefs = append(completeRefs, localData.Base.ID())
		datum := localData.Image
		id := getID(completeRefs, (*datum).Id)

		imageData, placeholder, errUC := processImage(dir, *localData.Image)
		if errUC != nil {
			log.Error().Msgf("  failed to process image '%+v', msg: %+v", id, errUC)
			continue
		}

		payload := &content.Attachment{
			Id:           id,
			RefIds:       completeRefs,
			OwnerId:      i.namespace,
			Name:         (*localData.Image).Url,
			Hash:         localHash[dir+"|"+(*localData.Image).Url],
			Description:  (*localData.Image).Description,
			Tags:         (*localData.Image).Tags,
			ImageDataUrl: placeholder,
			CreatedAt:    time.Now().Format(time.RFC3339),
		}
		log.Info().Msgf("PAYLOD: %+v", payload)
		ra, errUC := i.client.Upload(ctx, i.namespace, payload, "", imageData)
		if errUC != nil {
			log.Error().Msgf("  failed to sync an attachment in remote '%+v', msg: %+v", id, errUC)
			continue
		}

		// sync local data with remote
		*localData.Image = attachmentToImage(ra)
	}

	return stat, nil

}

func (i *imageDep[T]) computeImageConfigHashMulti(dir string, images map[string]ImageContextPair[T]) (map[string]string, []*types.Error) {
	id2hash := make(map[string]string)
	errUC := make([]*types.Error, 0)
	log.Info().Msgf("Read image path: %v", dir)
	for _, image := range images {
		imgHash, err := computeImageConfigHash(dir, *image.Image)
		if err != nil {
			errUC = append(errUC, &types.Error{
				HTTPCode: http.StatusBadRequest, Code: "CLIENT_ERROR", Message: "Cannot open image '" + (*image.Image).Url + "' or compute its hash.\n Make sure you entered a valid image at that path.\n error: " + err.Error(),
			})
			continue
		}
		id2hash[dir+"|"+(*image.Image).Url] = imgHash
	}

	return id2hash, errUC
}

// compute hash for the image config
func computeImageConfigHash(dir string, img *entity.Image) (string, error) {
	h := fnv.New128()

	url := path.Join(dir, img.Url)
	f, err := os.Open(url)
	if err != nil {
		return "", err
	}
	defer f.Close()

	imgData, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}

	// the raw image
	h.Write(imgData)

	num := make([]byte, 4, 4)
	binary.BigEndian.PutUint32(num, uint32(img.OffsetX))
	h.Write(num)
	binary.BigEndian.PutUint32(num, uint32(img.OffsetY))
	h.Write(num)
	binary.BigEndian.PutUint32(num, uint32(img.RatioX))
	h.Write(num)
	binary.BigEndian.PutUint32(num, uint32(img.RatioY))
	h.Write(num)
	binary.BigEndian.PutUint32(num, uint32(img.ScaleDirection))
	h.Write(num)
	binary.BigEndian.PutUint32(num, uint32(img.ScalePx))
	h.Write(num)
	binary.BigEndian.PutUint32(num, uint32(img.Rotation))
	h.Write(num)

	_result := make([]byte, 0, 16)
	_result = h.Sum(_result)

	result := base64.StdEncoding.EncodeToString(_result)

	return result, nil
}

func processImage(dir string, img *entity.Image) ([]byte, string, *types.CommonError) {
	id := getID((*img).RefIds, (*img).Id)
	url := path.Join(dir, img.Url)
	f, err := os.Open(url)
	if err != nil {
		return nil, "", &types.CommonError{
			Errors: []types.Error{
				{HTTPCode: http.StatusBadRequest, Code: "CLIENT_ERROR", Message: "Cannot open image '" + id + " ' at '" + url + "': " + err.Error()},
			},
		}
	}
	defer f.Close()

	buf, format, err := image.Decode(f)
	if err != nil {
		return nil, "", &types.CommonError{
			Errors: []types.Error{
				{HTTPCode: http.StatusBadRequest, Code: "CLIENT_ERROR", Message: "Cannot decode image. Make sure thhe image format '" + format + "' is wellknown. " + img.Id + " -> '" + img.Url + "': " + err.Error()},
			},
		}
	}

	if img.Rotation != 0 {
		buf = imaging.Rotate(buf, img.Rotation, image.Opaque)
	}

	// The processed & standarized image as png
	clean := imageproc.Crop(buf, int(img.OffsetX), int(img.OffsetY), int(img.RatioX), int(img.RatioY))

	// scale the image
	newWidth, newHeight := scaleParam(img, clean)

	scaled := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	draw.CatmullRom.Scale(scaled, scaled.Bounds(), clean, clean.Bounds(), draw.Over, nil)

	// dumps as PNG (later Webp)
	bbbuf := bytes.NewBuffer(make([]byte, 0))
	err = webp.Encode(bbbuf, scaled, nil)
	if err != nil {
		return nil, "", &types.CommonError{
			Errors: []types.Error{
				{HTTPCode: http.StatusBadRequest, Code: "CLIENT_ERROR", Message: "Cannot encode image as webp. Make sure you install webp driver. See more: 'https://github.com/kolesa-team/go-webp'. " + img.Id + " -> '" + img.Url + "': " + err.Error()},
			},
		}
	}

	// Calculate hash to make sure no redundant upload for same hash in the same ID
	data := bbbuf.Bytes()

	// for image_data_url (placeholder) ; notice, only 32px bounding rectangle
	placeholderEncode := ""
	newWidth, newHeight = scaleParam(&entity.Image{
		ScalePx:        16, // scale very small for blur placeholder
		ScaleDirection: entity.SCALE_DIRECTION_HORIZONTAL,
	}, clean)
	placeholder := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	draw.CatmullRom.Scale(placeholder, placeholder.Bounds(), clean, clean.Bounds(), draw.Over, nil)

	bbbuf2 := bytes.NewBuffer(make([]byte, 0))
	err = webp.Encode(bbbuf2, placeholder, nil)
	if err != nil {
		return data, placeholderEncode, &types.CommonError{
			Errors: []types.Error{
				{HTTPCode: http.StatusBadRequest, Code: "CLIENT_ERROR", Message: "Cannot encode image as webp. Make sure you install webp driver. See more: 'https://github.com/kolesa-team/go-webp'. " + img.Id + " -> '" + img.Url + "': " + err.Error()},
			},
		}
	}
	placeholderEncode = "data:image/webp;base64," + base64.StdEncoding.EncodeToString(bbbuf2.Bytes())

	return data, placeholderEncode, nil
}

func scaleParam(img *entity.Image, clean image.Image) (int, int) {
	target := int(img.ScalePx)
	axis := img.ScaleDirection
	if target == 0 {
		target = int(clean.Bounds().Dx())
		axis = entity.SCALE_DIRECTION_HORIZONTAL
	}
	scale := float64(target) / float64(clean.Bounds().Dx())
	newWidth := int(target)
	newHeight := int(float64(clean.Bounds().Dy()) * scale)

	if axis == entity.SCALE_DIRECTION_VERTICAL {
		scale = float64(target) / float64(clean.Bounds().Dy())
		newWidth = int(float64(clean.Bounds().Dx()) * scale)
		newHeight = target
	}
	return newWidth, newHeight
}

// attachmentToImage an utility function to map attachment to an entity
func attachmentToImage(attachment *content.Attachment) *entity.Image {
	return &entity.Image{
		Id:          attachment.Id,
		DataUrl:     attachment.ImageDataUrl,
		Url:         attachment.Url,
		Tags:        attachment.Tags,
		Description: attachment.Description,
		// Hash: r, should not have haash.. TODO: deprecate hash in thumbnail. Use in attachment instead
	}
}

func attachmentToMap(attachments []*entity.Attachment) map[string]*entity.Attachment {
	result := make(map[string]*entity.Attachment)
	for _, attachment := range attachments {
		id := getID(attachment.RefIDs(), attachment.ID())
		result[id] = attachment
	}
	return result
}
