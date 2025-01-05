package authapi

// type GSIData struct {
// 	Email         string `json:"email"` // also acts as an Id
// 	Url           string `json:"url"`
// 	Authorization map[string]Authorization
// 	OwnerId       string `json:"owner_id"`
// 	CreatedAt     string `json:"created_at"`
// }

// type Authorization struct {
// 	GroupId            string          `json:"group_id"`
// 	UserId             string          `json:"user_id"`
// 	Name               string          `json:"name"` // value???
// 	UiAndApiPermission map[string]bool `json:"api_and_ui_permission"`
// }

type (
	Authorization struct {
		UserID             string              `json:"user_id"` // organization/tenant id
		Name               string              `json:"name"`    // organization/tenant name
		UserGroupID        map[string]struct{} `json:"user_group_id"`
		UiAndApiPermission map[string]bool     `json:"api_and_ui_permission"`
	}

	Payload struct {
		Email           string                   `json:"email"` // email (as ID, possible improvements)
		Profile         UserProfile              `json:"profile"`
		GSI             GSIConfig                `json:"gsi"`
		MIP             MIPConfig                `json:"mip"`
		DefaultHomepage string                   `json:"default_homepage"`
		Authorization   map[string]Authorization `json:"authorization"` // organization/tenant id as key
	}

	UserProfile struct {
		ID               string `json:"id"`
		ImageURL         string `json:"image_url"`
		Name             string `json:"name"`
		DisplayName      string `json:"display_name"`
		Role             string `json:"role"`
		Description      string `json:"description"`
		Avatar1x1URL     string `json:"avatar_1x1_url"`
		Background3x1URL string `json:"background_3x1_url"`
		CreatedAt        string `json:"created_at"`
	}

	GSIConfig struct {
		Email string `json:"email"`
	}

	MIPConfig struct {
		Email string `json:"email"`
	}
)
