package enterpriseauth

import "context"

var Default Provider

type Provider interface {
	Get(ctx context.Context, email string) (map[string]Data, error)
	GetAll(ctx context.Context) (map[string]Organization, error)
}

type Auth string

const (
	AUTH_GSI Auth = "gsi"
)

type Data struct {
	Organization Organization `json:"organization"`
	Email        string       `json:"email"`
	Roles        []string     `json:"roles"`
	UserID       string       `json:"user_id"`
	GroupID      string       `json:"group_id"`
}

type Organization struct {
	URL    string `json:"url"`
	ApiURL string `json:"api_url"`

	// SignInPK should be the path to the actual key in GSM
	SignInPK    string `json:"sign_in_pk"`
	SignInKeyID string `json:"sign_in_key_id"`
	Auth        Auth   `json:"auth"`
}

func Get(ctx context.Context, email string) (map[string]Data, error) {
	if Default == nil {
		return nil, nil
	}
	return Default.Get(ctx, email)
}

func GetAll(ctx context.Context) (map[string]Organization, error) {
	if Default == nil {
		return nil, nil
	}
	return Default.GetAll(ctx)
}
