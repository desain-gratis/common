package authapiclient

import (
	"context"
	"fmt"
	"net/http"

	"golang.org/x/oauth2/google"

	types "github.com/desain-gratis/common/types/http"
)

// GetGoogleIDToken using application default credential
// you will get one if install gcloud-cli
func GetGoogleIDToken() (string, *types.CommonError) {
	// https://cloud.google.com/docs/authentication/get-id-token#go
	// but it's not working..
	// upon googling the solution.., I found the solution:
	// https://github.com/googleapis/google-api-go-client/issues/873
	gts, err := google.DefaultTokenSource(context.Background())
	if err != nil {
		fmt.Println(fmt.Errorf("failed to create DefaultTokenSource: %w", err))
		return "", &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusBadRequest,
					Code:     "FAILED_GET_GOOGLE_DEFAULT_TOKEN_SOURCE",
					Message:  "Cannot obtain token source from google client. Do you have gcloud-cli installed already? err: " + err.Error(),
				},
			},
		}
	}

	tok, err := gts.Token()
	if err != nil {
		return "", &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusBadRequest,
					Code:     "FAILED_GET_TOKEN",
					Message:  "Cannot obtain token from token source in google client. err: " + err.Error(),
				},
			},
		}
	}

	tokcer, ok := tok.Extra("id_token").(string)
	if !ok {
		return "", &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusBadRequest,
					Code:     "FAILED_GET_ID_TOKEN",
					Message:  "Google token error. err: " + err.Error(),
				},
			},
		}
	}

	return tokcer, nil
}
