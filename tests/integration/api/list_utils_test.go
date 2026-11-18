package api_test

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/harvester/harvester/tests/framework/helper"
)

// fetchList lists a collection of the API server. It returns the status code, the decoded
// response (only for status 200) and the raw body.
func fetchList(api, path string, query url.Values) (int, listResponse, string) {
	target := api
	if path != "" {
		target += "/" + path
	}
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	code, body, err := helper.GetResponse(target)
	MustNotError(err)
	var resp listResponse
	if code == http.StatusOK {
		MustNotError(json.Unmarshal(body, &resp))
	}
	return code, resp, string(body)
}

// ids returns the ids of the objects of the list, in the order of the response.
func (l listResponse) ids() []string {
	ids := make([]string, 0, len(l.Data))
	for _, item := range l.Data {
		ids = append(ids, item.ID)
	}
	return ids
}
