package odata

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
)

// FromResponse parses an http.Response and returns an unmarshalled OData
// If no odata is present in the response, or the content type is invalid, returns nil
func FromResponse(resp *http.Response) (*OData, error) {
	if resp == nil {
		return nil, nil
	}

	var o OData

	// Check for json content before looking for odata metadata
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.HasPrefix(contentType, "application/json") {
		// Read the response body and close it
		respBody, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("could not read response body: %s", err)
		}

		// Unmarshall odata
		if err := json.Unmarshal(respBody, &o); err != nil {
			return nil, err
		}

		// Reassign the response body
		resp.Body = ioutil.NopCloser(bytes.NewBuffer(respBody))

		return &o, nil
	}

	return nil, nil
}
