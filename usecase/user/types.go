package user

type (
	Details struct {
		ID              string                   `json:"id"` // email
		Profile         UserProfile              `json:"profile"`
		GSI             GSIConfig                `json:"gsi"`
		MIP             MIPConfig                `json:"mip"`
		DefaultHomepage string                   `json:"default_homepage"`
		Authorization   map[string]Authorization `json:"authorization"` // organization/tenant id as key
	}

	Authorization struct {
		UserID             string              `json:"user_id"` // organization/tenant id
		Name               string              `json:"name"`    // organization/tenant name
		UserGroupID        map[string]struct{} `json:"user_group_id"`
		UiAndApiPermission map[string]bool     `json:"api_and_ui_permission"`
	}

	Payload struct {
		ID              string                   `json:"id"` // email
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
