package mycontentapi

import (
	"net/url"

	"github.com/desain-gratis/common/usecase/mycontent"
)

// URLFormat for custom URL (this should be the URL default)
type URLFormat func(baseURL string, userID string, refIDs []string, ID string) string

// FormatURL inplace
func FormatURL[T mycontent.Data](baseURL string, params []string) func(t T) {
	return func(t T) {
		refIDs := t.RefIDs()
		u, _ := url.Parse(baseURL)
		q := make(url.Values)
		for idx, param := range params {
			q[param] = []string{refIDs[idx]}
		}
		q["id"] = []string{t.ID()}

		u.RawQuery = q.Encode()
		t.WithURL(u.String())
	}
}
