package repository

const CREATED = "CREATED"
const UPDATED = "UPDATED"
const DELETED = "DELETED"

type MyError struct {
	Message string
}

func (m *MyError) Error() string {
	return m.Message
}

type (
	AuthorizedUser struct {
		ID              string                   `json:"id"` // email
		Profile         UserProfile              `json:"profile"`
		GSI             GSIConfig                `json:"gsi"`
		MIP             MIPConfig                `json:"mip"`
		DefaultHomepage string                   `json:"default_homepage"`
		Authorization   map[string]Authorization `json:"authorization"` // organization/tenant id as key
	}

	UserGroup struct {
		ID             string              `json:"id"`
		OwnerID        string              `json:"owner_id"`
		OrganizationID string              `json:"organization_id"`
		DisplayURL     string              `json:"display_url"`
		Name           string              `json:"name"`
		DisplayName    string              `json:"display_name"`
		Description    string              `json:"description"`
		Caption        string              `json:"caption"`
		Origin         string              `json:"origin"` // internal or external
		Icon           string              `json:"icon"`
		Color          string              `json:"color"`
		Avatar1x1      string              `json:"avatar"`
		Background3x1  string              `json:"background"`
		Members        map[string]struct{} `json:"members"`
		CreatedAt      string              `json:"created_at"`
	}

	Authorization struct {
		UserID             string              `json:"user_id"` // organization/tenant id
		Name               string              `json:"name"`    // organization/tenant name
		UserGroupID        map[string]struct{} `json:"user_group_id"`
		UiAndApiPermission map[string]bool     `json:"api_and_ui_permission"`
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
