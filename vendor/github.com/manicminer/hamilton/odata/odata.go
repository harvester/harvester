package odata

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/hashicorp/go-uuid"
)

const ODataVersion = "4.0" // TODO: support 4.01 - https://docs.oasis-open.org/odata/odata-json-format/v4.01/cs01/odata-json-format-v4.01-cs01.html#_Toc499720587

type Id string

func (o Id) MarshalJSON() ([]byte, error) {
	id := regexp.MustCompile(`/v2/`).ReplaceAllString(string(o), `/v1.0/`)

	u, err := url.Parse(id)
	if err != nil || u.Scheme == "" || u.Host == "" {
		matches := regexp.MustCompile(`([^()'"]+)\(['"]([^'"]+)['"]\)`).FindStringSubmatch(id)
		if len(matches) != 3 {
			return nil, fmt.Errorf("Marshaling odata.Id: could not match a GUID")
		}

		objectType := matches[1]
		guid := matches[2]
		if _, err = uuid.ParseUUID(guid); err != nil {
			return nil, fmt.Errorf("Marshaling odata.Id: %+v", err)
		}

		// Although we're hard-coding `graph.microsoft.com` here, this doesn't _appear_ to be a problem
		// The host can seemingly be anything, even complete nonsense, and the API will accept it provided
		// it can parse out a version number, an object type and a GUID.
		id = fmt.Sprintf("https://graph.microsoft.com/v1.0/%s/%s", objectType, guid)
	}

	return json.Marshal(id)
}

func (o *Id) UnmarshalJSON(data []byte) error {
	var id string
	if err := json.Unmarshal(data, &id); err != nil {
		return err
	}
	*o = Id(regexp.MustCompile(`/v2/`).ReplaceAllString(id, `/v1.0/`))

	return nil
}

type Link string

func (o *Link) UnmarshalJSON(data []byte) error {
	var link string
	if err := json.Unmarshal(data, &link); err != nil {
		return err
	}
	*o = Link(regexp.MustCompile(`/v2/`).ReplaceAllString(link, `/v1.0/`))
	return nil
}

// OData is used to unmarshall OData metadata from an API response.
type OData struct {
	Context      *string `json:"@odata.context"`
	MetadataEtag *string `json:"@odata.metadataEtag"`
	Type         *Type   `json:"@odata.type"`
	Count        *int    `json:"@odata.count"`
	NextLink     *string `json:"@odata.nextLink"`
	Delta        *string `json:"@odata.delta"`
	DeltaLink    *string `json:"@odata.deltaLink"`
	Id           *Id     `json:"@odata.id"`
	EditLink     *Link   `json:"@odata.editLink"`
	Etag         *string `json:"@odata.etag"`

	Error *Error `json:"-"`

	Value interface{} `json:"value"`
}

func (o *OData) UnmarshalJSON(data []byte) error {
	// Perform unmarshalling using a local type
	type odata OData
	var o2 odata
	if err := json.Unmarshal(data, &o2); err != nil {
		return err
	}
	*o = OData(o2)

	// Look for errors in the "error" and "odata.error" fields
	var e map[string]json.RawMessage
	if err := json.Unmarshal(data, &e); err != nil {
		return err
	}
	for _, k := range []string{"error", "odata.error"} {
		if v, ok := e[k]; ok {
			var e2 Error
			if err := json.Unmarshal(v, &e2); err != nil {
				return err
			}
			o.Error = &e2
			break
		}
	}
	return nil
}

// Error is used to unmarshal an API error message.
type Error struct {
	Code            *string          `json:"code"`
	Date            *string          `json:"date"`
	Message         *string          `json:"-"`
	RawMessage      *json.RawMessage `json:"message"` // sometimes a string, sometimes an object :/
	ClientRequestId *string          `json:"client-request-id"`
	RequestId       *string          `json:"request-id"`

	InnerError *Error `json:"innerError"` // nested errors

	Details *[]struct {
		Code   *string `json:"code"`
		Target *string `json:"target"`
	} `json:"details"`

	Values *[]struct {
		Item  string `json:"item"`
		Value string `json:"value"`
	} `json:"values"`
}

func (e *Error) UnmarshalJSON(data []byte) error {
	// Perform unmarshalling using a local type
	type error Error
	var e2 error
	if err := json.Unmarshal(data, &e2); err != nil {
		return err
	}
	*e = Error(e2)

	// Unmarshal the message, which can be a plain string or an object wrapping a message
	if raw := e.RawMessage; raw != nil && len(*raw) > 0 {
		switch string((*raw)[0]) {
		case "\"":
			var s string
			if err := json.Unmarshal(*raw, &s); err != nil {
				return err
			}
			e.Message = &s
		case "{":
			var m map[string]interface{}
			if err := json.Unmarshal(*raw, &m); err != nil {
				return err
			}
			if v, ok := m["value"]; ok {
				if s, ok := v.(string); ok {
					e.Message = &s
				}
			}
		default:
			return fmt.Errorf("unrecognised error message: %#v", string(*raw))
		}
	}
	return nil
}

func (e Error) String() string {
	sl := make([]string, 0)
	if e.Code != nil {
		sl = append(sl, *e.Code)
	}
	if e.Message != nil {
		sl = append(sl, *e.Message)
	}
	if e.InnerError != nil {
		if is := e.InnerError.String(); is != "" {
			sl = append(sl, is)
		}
	}
	return strings.Join(sl, ": ")
}

func (e Error) Match(errorText string) bool {
	re := regexp.MustCompile(errorText)
	return re.MatchString(e.String())
}
