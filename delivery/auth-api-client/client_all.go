package authapiclient

type allClient struct {
	GoogleAdmin *client
	Google      *client
	Microsoft   *client
}

func NewAll(
	adminEndpoint string,
	googleEndpoint string,
	microsoftEndpoint string,
) *allClient {
	return &allClient{
		GoogleAdmin: New(adminEndpoint),
		Google:      New(googleEndpoint),
		Microsoft:   New(microsoftEndpoint),
	}
}
