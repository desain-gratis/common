package mycontent

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
type Attachment struct {
	client[*entity.Attachment]
}

// Upload attachment
func (a *Attachment) Upload(ctx context.Context, metadata *entity.Attachment, fromPath string, fromMemory []byte) (authResp *entity.Attachment, errUC *types.CommonError) {
	reqbody, writer := io.Pipe()
	mwriter := multipart.NewWriter(writer)
	defer reqbody.Close()

	req, err := http.NewRequest(http.MethodPut, a.endpoint, reqbody)
	if err != nil {
		return nil, &types.CommonError{
			Errors: []types.Error{
				{Code: "CLIENT_ERROR", Message: "new request " + err.Error()},
			},
		}
	}
	req = req.WithContext(ctx)

	req.Header.Add("Content-Type", mwriter.FormDataContentType())
	req.Header.Add("Authorization", "Bearer "+a.token)
	req.Header.Add("X-User-Id", a.userID)
	req.Header.Add("X-Tenant-Id", a.tenantID)

	errChan := make(chan types.CommonError)

	go func() {
		defer close(errChan)
		defer writer.Close()
		defer mwriter.Close()

		// Write document metadata
		log.Debug().Msgf("Created form `document`..")
		documentW, err := mwriter.CreateFormField("document")
		if err != nil {
			errChan <- types.CommonError{
				Errors: []types.Error{
					{Code: "CLIENT_ERROR", Message: "CreateFormField metadata `doc` " + err.Error()},
				},
			}
			return
		}

		payload, err := json.Marshal(metadata)
		if err != nil {
			errChan <- types.CommonError{
				Errors: []types.Error{
					{Code: "CLIENT_ERROR", Message: "Marshal metadata `doc` " + err.Error()},
				},
			}
			return
		}

		log.Debug().Msgf("Write payload to `document`..")
		_, err = documentW.Write(payload)
		if err != nil {
			errChan <- types.CommonError{
				Errors: []types.Error{
					{Code: "CLIENT_ERROR", Message: "Write metadata `doc` " + err.Error()},
				},
			}
			return
		}

		// Using file
		if fromPath != "" {
			log.Debug().Msgf("Created form `attachment` using file")
			attachW, err := mwriter.CreateFormFile("attachment", fromPath)
			if err != nil {
				errChan <- types.CommonError{
					Errors: []types.Error{
						{Code: "CLIENT_ERROR", Message: "CreateFormFile `attachment` " + err.Error()},
					},
				}
				return
			}

			log.Debug().Msgf("Opened file at `%v`..", fromPath)
			f, err := os.Open(fromPath)
			if err != nil {
				errChan <- types.CommonError{
					Errors: []types.Error{
						{Code: "CLIENT_ERROR", Message: "Open `attachment` " + err.Error()},
					},
				}
				return
			}
			defer f.Close()

			log.Debug().Msgf("Uploading... ")
			_, err = io.Copy(attachW, f)
			if err != nil {
				errChan <- types.CommonError{
					Errors: []types.Error{
						{Code: "CLIENT_ERROR", Message: "Copy `attachment` " + err.Error()},
					},
				}
				return
			}
		} else {
			// Using memory buffer

			log.Debug().Msgf("Created form `attachment` using in-memory buffer..")
			h := &textproto.MIMEHeader{}
			h.Set("Content-Type", http.DetectContentType(fromMemory))
			h.Set("Content-Disposition", `name="attachment"`)
			documentW, err := mwriter.CreatePart(*h)
			if err != nil {
				errChan <- types.CommonError{
					Errors: []types.Error{
						{Code: "CLIENT_ERROR", Message: "CreateFormField `attachment` " + err.Error()},
					},
				}
				return
			}

			log.Debug().Msgf("Write payload to `attachment`..")
			n, err := documentW.Write(fromMemory)
			if err != nil {
				errChan <- types.CommonError{
					Errors: []types.Error{
						{Code: "CLIENT_ERROR", Message: "Write metadata `attachment` " + err.Error()},
					},
				}
				return
			}
			log.Debug().Msgf("Written %v bytes of data", n)
		}
		log.Debug().Msgf("Finished writing data..") // why it's immediate ya? might be going to network buffer directly?

	}()

	log.Debug().Msgf("  Request initiated!")

	log.Debug().Msgf("Doing....") // why it's immediate ya? might be going to network buffer directly?
	resp, err := a.httpc.Do(req)
	if err != nil {
		return nil, &types.CommonError{
			Errors: []types.Error{
				{Code: "CLIENT_ERROR", Message: "Do " + err.Error()},
			},
		}
	}

	log.Debug().Msgf("Waiting for response...")
	_errUC, ok := <-errChan // ok is false if there are no more values to receive and the channel is closed. https://go.dev/tour/concurrency/4
	if ok {
		// if channel is not closed (ok = true), then it's an error
		return nil, &_errUC
	}

	log.Debug().Msgf("Reading back response..")
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &types.CommonError{
			Errors: []types.Error{
				{Code: "CLIENT_ERROR", Message: "ReadAll " + err.Error()},
			},
		}
	}
	log.Debug().Msgf("Finished uploading!")

	var parsed types.CommonResponseTyped[entity.Attachment]
	err = json.Unmarshal(body, &parsed)
	if err != nil {
		return nil, &types.CommonError{
			Errors: []types.Error{
				{Code: "CLIENT_ERROR", Message: "unmarshall " + err.Error() + " " + string(body)},
			},
		}
	}

	if parsed.Error != nil {
		return nil, parsed.Error
	}

	return &parsed.Success, nil
}
