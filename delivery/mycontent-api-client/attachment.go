package mycontentapiclient

import (
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"

	"github.com/rs/zerolog/log"

	"github.com/desain-gratis/common/types/entity"
	types "github.com/desain-gratis/common/types/http"
)

// Attachment is a specific type of client
type attachmentClient struct {
	client[*entity.Attachment]
}

func NewAttachment(
	httpc *http.Client,
	endpoint string,
	refsParam []string,
) *attachmentClient {
	return &attachmentClient{
		client: client[*entity.Attachment]{
			httpc:     httpc,
			endpoint:  endpoint,
			refsParam: refsParam,
		},
	}
}

// Upload attachment
func (a *attachmentClient) Upload(ctx context.Context, authToken string, namespace string, metadata *entity.Attachment, fromPath string, fromMemory []byte) (authResp *entity.Attachment, errUC *types.CommonError) {
	reqbody, writer := io.Pipe()
	mwriter := multipart.NewWriter(writer)
	defer reqbody.Close()

	req, err := http.NewRequest(http.MethodPost, a.endpoint, reqbody)
	if err != nil {
		return nil, &types.CommonError{
			Errors: []types.Error{
				{Code: "CLIENT_ERROR", Message: "new request " + err.Error()},
			},
		}
	}
	req = req.WithContext(ctx)

	if authToken != "" {
		req.Header.Add("Authorization", "Bearer "+authToken)
	}

	req.Header.Add("X-Namespace", namespace)
	req.Header.Add("Content-Type", mwriter.FormDataContentType())

	errChan := make(chan *types.CommonError)

	go func() {
		defer close(errChan)
		defer writer.Close()
		defer mwriter.Close()

		// Write document metadata
		log.Debug().Msgf("created form `document`..")
		documentW, err := mwriter.CreateFormField("document")
		if err != nil {
			errChan <- &types.CommonError{
				Errors: []types.Error{
					{Code: "CLIENT_ERROR", Message: "CreateFormField metadata `doc` " + err.Error()},
				},
			}
			return
		}

		payload, err := json.Marshal(metadata)
		if err != nil {
			errChan <- &types.CommonError{
				Errors: []types.Error{
					{Code: "CLIENT_ERROR", Message: "Marshal metadata `doc` " + err.Error()},
				},
			}
			return
		}

		log.Debug().Msgf("write payload to `document`..")
		_, err = documentW.Write(payload)
		if err != nil {
			errChan <- &types.CommonError{
				Errors: []types.Error{
					{Code: "CLIENT_ERROR", Message: "Write metadata `doc` " + err.Error()},
				},
			}
			return
		}

		// Using file
		if fromPath != "" {
			log.Debug().Msgf("created form `attachment` using file")
			attachW, err := mwriter.CreateFormFile("attachment", fromPath)
			if err != nil {
				errChan <- &types.CommonError{
					Errors: []types.Error{
						{Code: "CLIENT_ERROR", Message: "CreateFormFile `attachment` " + err.Error()},
					},
				}
				return
			}

			log.Debug().Msgf("opened file at `%v`..", fromPath)
			f, err := os.Open(fromPath)
			if err != nil {
				errChan <- &types.CommonError{
					Errors: []types.Error{
						{Code: "CLIENT_ERROR", Message: "Open `attachment` " + err.Error()},
					},
				}
				return
			}
			defer f.Close()

			log.Debug().Msgf("uploading... ")
			_, err = io.Copy(attachW, f)
			if err != nil {
				errChan <- &types.CommonError{
					Errors: []types.Error{
						{Code: "CLIENT_ERROR", Message: "Copy `attachment` " + err.Error()},
					},
				}
				return
			}
		} else {
			// Using memory buffer

			h := &textproto.MIMEHeader{}
			h.Set("Content-Type", http.DetectContentType(fromMemory))
			h.Set("Content-Disposition", `form-data; name="attachment"`)

			log.Debug().Msgf("created form `attachment` using in-memory buffer..")
			documentW, err := mwriter.CreatePart(*h)
			if err != nil {
				errChan <- &types.CommonError{
					Errors: []types.Error{
						{Code: "CLIENT_ERROR", Message: "CreateFormField `attachment` " + err.Error()},
					},
				}
				return
			}

			log.Debug().Msgf("write payload to `attachment`..")
			n, err := documentW.Write(fromMemory)
			if err != nil {
				errChan <- &types.CommonError{
					Errors: []types.Error{
						{Code: "CLIENT_ERROR", Message: "Write metadata `attachment` " + err.Error()},
					},
				}
				return
			}
			log.Debug().Msgf("written %v bytes of data is OK: %v", n, n == len(fromMemory))
		}
		log.Debug().Msgf("finished writing data..") // why it's immediate ya? might be going to network buffer directly?
	}()

	log.Debug().Msgf("request initiated!")

	log.Debug().Msgf("doing....") // why it's immediate ya? might be going to network buffer directly?
	resp, err := a.httpc.Do(req)
	if err != nil {
		return nil, &types.CommonError{
			Errors: []types.Error{
				{Code: "CLIENT_ERROR", Message: "Do " + err.Error()},
			},
		}
	}

	log.Debug().Msgf("waiting for response")
	_errUC, ok := <-errChan // ok is false if there are no more values to receive and the channel is closed. https://go.dev/tour/concurrency/4
	if ok || _errUC != nil {
		// if channel is not closed (ok = true), then it's an error
		return nil, _errUC
	}

	log.Debug().Msgf("reading back response")
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &types.CommonError{
			Errors: []types.Error{
				{Code: "CLIENT_ERROR", Message: "ReadAll " + err.Error()},
			},
		}
	}
	log.Debug().Msgf("finished uploading!")

	var parsed types.CommonResponseTyped[entity.Attachment]
	err = json.Unmarshal(body, &parsed)
	if err != nil {
		return nil, &types.CommonError{
			Errors: []types.Error{
				{Code: "CLIENT_ERROR", Message: "unmarshal " + err.Error() + " " + string(body)},
			},
		}
	}

	if parsed.Error != nil {
		return nil, parsed.Error
	}

	return &parsed.Success, nil
}
