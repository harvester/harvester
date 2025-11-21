package util

// Structs for generating Kubernetes PATCH requests with json.Marshal
// Generating the patch request by serializing a struct guarantees that the JSON
// data in the request will not be malformed.
//
// Utilize like this:
//
// op := json.Marshall(
//   PatchStringValue{
//     Op:    "replace",
//     Path:  "/spec/property",
//     Value: "newValue",
//   },
// )

type PatchStringValue struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value string `json:"value"`
}
