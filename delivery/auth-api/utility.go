package authapi

import (
	"github.com/desain-gratis/common/usecase/user"
)

func convertUpdatePayload(clientPayload Payload) (ucPayload user.Payload) {
	auths := make(map[string]user.Authorization)
	for tenantID, auth := range clientPayload.Authorization {
		auths[tenantID] = user.Authorization{
			UserID:             auth.UserID,
			Name:               auth.Name,
			UserGroupID:        auth.UserGroupID,
			UiAndApiPermission: auth.UiAndApiPermission,
		}
	}
	ucPayload = user.Payload{
		// registering based on email ID (in github config)
		// later improvemetns:
		//   for Google ID, should use "sub" field later improvement
		//   but user need to signin first to create the account
		Id:              clientPayload.Email,
		Profile:         user.UserProfile(clientPayload.Profile),
		GSI:             user.GSIConfig(clientPayload.GSI),
		MIP:             user.MIPConfig(clientPayload.MIP),
		DefaultHomepage: clientPayload.DefaultHomepage,
		Authorization:   auths,
	}
	return
}
