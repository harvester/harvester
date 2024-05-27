package msgraph

import "encoding/json"

func MarshalDocs(docs [][]byte) ([]byte, error) {
	out := make(map[string]interface{})
	for _, d := range docs {
		var o map[string]interface{}
		err := json.Unmarshal(d, &o)
		if err != nil {
			return d, err
		}
		for k, v := range o {
			out[k] = v
		}
	}
	return json.Marshal(out)
}
