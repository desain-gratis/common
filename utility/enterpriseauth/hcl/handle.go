package hcl

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/rs/zerolog/log"

	"github.com/desain-gratis/common/utility/enterpriseauth"
	enterpiseauth "github.com/desain-gratis/common/utility/enterpriseauth"
)

var _ enterpiseauth.Provider = &handler{}

type handler struct {
	// map[email]map[organization]
	data map[string]map[string]enterpiseauth.Data
	org  map[string]enterpriseauth.Organization
}

type Config struct {
	Organization []Organization `hcl:"organization,block"`
}

type Organization struct {
	URL         string   `hcl:"url"`
	Auth        string   `hcl:"auth"`
	SignInPK    string   `hcl:"sign_in_pk"`
	SignInKeyID string   `hcl:"sign_in_key_id"`
	ApiURL      string   `hcl:"api_url"`
	Member      []Member `hcl:"member,block"`
}

type Member struct {
	Email  string   `hcl:"email"`
	Roles  []string `hcl:"roles"`
	UserID string   `hcl:"user_id"`
}

// New
func New(glob string) *handler {
	fileNames, err := os.ReadDir(glob)
	if err != nil {
		log.Err(err).Msgf("Error reading from files")
	}

	var configs []*hcl.File

	for _, entry := range fileNames {
		filename := entry.Name()
		log.Debug().Msgf("Reading roles config %v", filename)
		suffix := strings.ToLower(filepath.Ext(filename))
		if suffix != ".hcl" || entry.IsDir() {
			continue
		}

		p := filepath.Join(glob, filename)

		f, err := os.ReadFile(p)
		if err != nil {
			log.Err(err).Msgf("Error reading from file %v", filename)
			continue
		}

		file, diags := hclsyntax.ParseConfig(f, filename, hcl.Pos{Line: 1, Column: 1})
		if diags.HasErrors() {
			for _, err := range diags.Errs() {
				log.Err(err).Msgf("Error reading from file %v", filename)
			}
			continue
		}
		configs = append(configs, file)
	}

	body := hcl.MergeFiles(configs)

	var target Config
	diags := gohcl.DecodeBody(body, nil, &target)
	for _, err := range diags.Errs() {
		log.Err(err).Msgf("Error reading authorization config.")
	}

	cacheOrg := make(map[string]enterpiseauth.Organization)
	cache := make(map[string]map[string]enterpiseauth.Data)
	for _, org := range target.Organization {
		cacheOrg[org.URL] = enterpiseauth.Organization{
			URL:         org.URL,
			SignInPK:    org.SignInPK,
			SignInKeyID: org.SignInKeyID,
			Auth:        enterpiseauth.Auth(org.Auth),
			ApiURL:      org.ApiURL,
		}
		for _, mem := range org.Member {
			if _, ok := cache[mem.Email]; !ok {
				cache[mem.Email] = make(map[string]enterpiseauth.Data)
			}
			cache[mem.Email][org.URL] = enterpiseauth.Data{
				Organization: enterpiseauth.Organization{
					URL:         org.URL,
					SignInPK:    org.SignInPK,
					SignInKeyID: org.SignInKeyID,
					Auth:        enterpiseauth.Auth(org.Auth),
					ApiURL:      org.ApiURL,
				},
				Email:  mem.Email,
				Roles:  mem.Roles,
				UserID: mem.UserID,
			}
		}
	}

	return &handler{
		data: cache,
		org:  cacheOrg,
	}
}

func (h *handler) Get(ctx context.Context, email string) (map[string]enterpiseauth.Data, error) {
	return h.data[email], nil
}

// GetAll please don't modify
func (h *handler) GetAll(ctx context.Context) (map[string]enterpiseauth.Organization, error) {
	return h.org, nil
}
