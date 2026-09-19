package mycontentapi

import (
	"net/url"

	"github.com/desain-gratis/common/delivery/mycontent-api/mycontent"
)

// URLFormat for custom URL (this should be the URL default)
type URLFormat func(baseURL string, userID string, refIDs []string, ID string) string

// FormatURL inplace
func FormatURL[T mycontent.Data](baseURL string, params []string) func(t T) {
	return func(t T) {
		u, err := url.Parse(baseURL)
		if err != nil {
			return
		}

		q := make(url.Values)
		q["id"] = []string{t.ID()}

		refIDs := t.RefIDs()
		if refIDs != nil && len(refIDs) == len(params) {
			for idx, param := range params {
				q[param] = []string{refIDs[idx]}
			}
		}

		u.RawQuery = q.Encode()
		t.WithURL(u.String())
	}
}
