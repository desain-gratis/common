package handler

import (
	"context"
	"strings"

	mRepo "github.com/desain-gratis/common/repository"
	m "github.com/desain-gratis/common/usecase/user"
)

func (u *usecase) GetByOrgID(ctx context.Context, organizationID, nameContained string) (result []m.Details, err error) {
	resp, err := u.userPGRepo.Get(ctx, "root", "", []string{})
	if err != nil {
		return
	}

	for _, user := range resp {
		// if user is not in organization id, then skip
		if _, ok := user.Authorization[organizationID]; !ok {
			continue
		}

		// if need to be filtered by name contained, and the user name doesn't match, then skip
		if nameContained != "" && (!strings.Contains(strings.ToLower(user.Profile.Name), strings.ToLower(nameContained)) &&
			!strings.Contains(strings.ToLower(user.Profile.DisplayName), strings.ToLower(nameContained))) {
			continue
		}

		auths := make(map[string]m.Authorization)
		for tenantID, auth := range user.Authorization {
			auths[tenantID] = m.Authorization{
				UserID:             auth.UserID,
				Name:               auth.Name,
				UserGroupID:        auth.UserGroupID,
				UiAndApiPermission: auth.UiAndApiPermission,
			}
		}

		result = append(result, m.Details{
			ID: user.ID,
			Profile: m.UserProfile{
				ID:               user.Profile.ID,
				ImageURL:         user.Profile.ImageURL,
				Name:             user.Profile.Name,
				DisplayName:      user.Profile.DisplayName,
				Role:             user.Profile.Role,
				Description:      user.Profile.Description,
				Avatar1x1URL:     user.Profile.Avatar1x1URL,
				Background3x1URL: user.Profile.Background3x1URL,
				CreatedAt:        user.Profile.CreatedAt,
			},
			GSI:             m.GSIConfig{Email: user.GSI.Email},
			MIP:             m.MIPConfig{Email: user.MIP.Email},
			DefaultHomepage: user.DefaultHomepage,
			Authorization:   auths,
		})
	}
	return
}

func (u *usecase) GetDetail(ctx context.Context, email string) (result m.Details, err error) {
	resp, err := u.userPGRepo.Get(ctx, "root", email, []string{})
	if err != nil {
		return
	}

	if len(resp) > 0 {
		user := resp[0]

		auths := make(map[string]m.Authorization)
		for tenantID, auth := range user.Authorization {
			auths[tenantID] = m.Authorization{
				UserID:             auth.UserID,
				Name:               auth.Name,
				UserGroupID:        auth.UserGroupID,
				UiAndApiPermission: auth.UiAndApiPermission,
			}
		}
		result = m.Details{
			ID: user.ID,
			Profile: m.UserProfile{
				ID:               user.Profile.ID,
				ImageURL:         user.Profile.ImageURL,
				Name:             user.Profile.Name,
				DisplayName:      user.Profile.DisplayName,
				Role:             user.Profile.Role,
				Description:      user.Profile.Description,
				Avatar1x1URL:     user.Profile.Avatar1x1URL,
				Background3x1URL: user.Profile.Background3x1URL,
				CreatedAt:        user.Profile.CreatedAt,
			},
			GSI:             m.GSIConfig{Email: user.GSI.Email},
			MIP:             m.MIPConfig{Email: user.MIP.Email},
			DefaultHomepage: user.DefaultHomepage,
			Authorization:   auths,
		}
	}
	return
}

func (u *usecase) Insert(ctx context.Context, payload m.Payload) (err error) {
	auths := make(map[string]mRepo.Authorization)
	for tenantID, auth := range payload.Authorization {
		auths[tenantID] = mRepo.Authorization{
			UserID:             auth.UserID,
			Name:               auth.Name,
			UserGroupID:        auth.UserGroupID,
			UiAndApiPermission: auth.UiAndApiPermission,
		}
	}
	userPayload := mRepo.AuthorizedUser{
		ID: payload.Id,
		Profile: mRepo.UserProfile{
			ID:               payload.Profile.ID,
			ImageURL:         payload.Profile.ImageURL,
			Name:             payload.Profile.Name,
			DisplayName:      payload.Profile.DisplayName,
			Role:             payload.Profile.Role,
			Description:      payload.Profile.Description,
			Avatar1x1URL:     payload.Profile.Avatar1x1URL,
			Background3x1URL: payload.Profile.Background3x1URL,
			CreatedAt:        payload.Profile.CreatedAt,
		},
		GSI:             mRepo.GSIConfig{Email: payload.GSI.Email},
		MIP:             mRepo.MIPConfig{Email: payload.MIP.Email},
		DefaultHomepage: payload.DefaultHomepage,
		Authorization:   auths,
	}

	err = u.userPGRepo.Insert(ctx, "root", payload.Id, []string{}, userPayload)
	return
}

func (u *usecase) Update(ctx context.Context, email string, payload m.Payload) (err error) {
	auths := make(map[string]mRepo.Authorization)
	for tenantID, auth := range payload.Authorization {
		auths[tenantID] = mRepo.Authorization{
			UserID:             auth.UserID,
			Name:               auth.Name,
			UserGroupID:        auth.UserGroupID,
			UiAndApiPermission: auth.UiAndApiPermission,
		}
	}
	userPayload := mRepo.AuthorizedUser{
		ID: payload.Id,
		Profile: mRepo.UserProfile{
			ID:               payload.Profile.ID,
			ImageURL:         payload.Profile.ImageURL,
			Name:             payload.Profile.Name,
			DisplayName:      payload.Profile.DisplayName,
			Role:             payload.Profile.Role,
			Description:      payload.Profile.Description,
			Avatar1x1URL:     payload.Profile.Avatar1x1URL,
			Background3x1URL: payload.Profile.Background3x1URL,
			CreatedAt:        payload.Profile.CreatedAt,
		},
		GSI:             mRepo.GSIConfig{Email: payload.GSI.Email},
		MIP:             mRepo.MIPConfig{Email: payload.MIP.Email},
		DefaultHomepage: payload.DefaultHomepage,
		Authorization:   auths,
	}

	err = u.userPGRepo.Update(ctx, "root", email, []string{}, userPayload)
	return
}

func (u *usecase) Delete(ctx context.Context, email string) (err error) {
	err = u.userPGRepo.Delete(ctx, "root", email, []string{})
	return
}
