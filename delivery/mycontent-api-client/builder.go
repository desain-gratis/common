package mycontentapiclient

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"path"

	"github.com/desain-gratis/common/delivery/mycontent-api/mycontent"
	"github.com/desain-gratis/common/types/entity"
)

type builder[T mycontent.Data] struct {
	endpoint *url.URL

	syncer *sync[T]

	imageDir string
	fileDir  string
}

func Builder[T mycontent.Data](endpoint *url.URL, params ...string) *builder[T] {
	return &builder[T]{
		endpoint: endpoint,
		syncer: &sync[T]{
			client: &client[T]{
				httpc:     http.DefaultClient,
				endpoint:  endpoint.String(),
				refsParam: params,
			},
			namespace: "*",
			data:      nil,
			OptConfig: OptionalConfig{},
		},
	}
}

func (b *builder[T]) WithClient(client *http.Client) *builder[T] {
	b.syncer.client.httpc = client
	return b
}

func (b *builder[T]) WithNamespace(ns string) *builder[T] {
	b.syncer.namespace = ns
	return b
}

func (b *builder[T]) WithData(data []T) *builder[T] {
	b.syncer.data = data
	return b
}

func (b *builder[T]) WithAuth(token string) *builder[T] {
	b.syncer.OptConfig.AuthorizationToken = token
	return b
}

// actually dep is also kind of a builder

func (i *imageDep[T]) WithUploadDirectory(dir string) *imageDep[T] {
	i.uploadDir = dir
	return i
}

func (i *imageDep[T]) WithCustomPath(pathFun func(T) string) *imageDep[T] {
	i.customPath = pathFun
	return i
}

func (i *fileDep[T]) WithUploadDirectory(dir string) *fileDep[T] {
	i.uploadDir = dir
	return i
}

func (i *fileDep[T]) WithCustomPath(pathFun func(T) string) *fileDep[T] {
	// TODO: actually implement
	i.customPath = pathFun
	return i
}

func (b *builder[T]) WithImages(extract ExtractImages[T], relPath string, params ...string) *imageDep[T] {
	imageEndpoint := *b.endpoint
	imageEndpoint.Path = path.Join(imageEndpoint.Path, relPath)

	dep := &imageDep[T]{
		extract: extract,
		client: &attachmentClient{
			client: client[*entity.Attachment]{
				endpoint:  imageEndpoint.String(),
				refsParam: append(b.syncer.client.refsParam, params...), // parent refs param
				httpc:     b.syncer.client.httpc,
			},
		},
		sync:       b.syncer,
		uploadDir:  b.imageDir,
		customPath: nil,
	}
	// can spawn go routine pool

	b.syncer.imageDeps = append(b.syncer.imageDeps, dep)

	// so user can modify/configure separately
	return dep
}

// TODO: move to mycontent-api
type AttachmentUploader[T mycontent.Data] interface {
	// upload context
	Upload(ctx context.Context, path string, data T) (io.ReadCloser, *entity.Attachment, error)
}

func (b *builder[T]) WithFiles(extract ExtractFiles[T], relPath string, params ...string) *fileDep[T] {
	filesEndpoint := *b.endpoint
	filesEndpoint.Path = path.Join(filesEndpoint.Path, relPath)

	dep := &fileDep[T]{
		extract: extract,
		client: &attachmentClient{
			client: client[*entity.Attachment]{
				endpoint:  filesEndpoint.String(),
				refsParam: params,
				httpc:     b.syncer.client.httpc,
			},
		},
		sync:       b.syncer,
		uploadDir:  b.fileDir,
		customPath: nil,
	}
	b.syncer.fileDeps = append(b.syncer.fileDeps, dep)

	// so user can modify/configure separately
	return dep
}

func (b *builder[T]) WithDependency(extract Extract) {}

type Extractd[T mycontent.Data] func(T) []mycontent.Data
type ExtractAttachment[T mycontent.Data] func(T) []

func (b *builder[T]) Build() *sync[T] {
	if b.syncer.namespace == "" {
		panic("empty namespace")
	}

	if b.syncer.client.endpoint == "" {
		panic("empty endpoint")
	}

	return b.syncer
}
