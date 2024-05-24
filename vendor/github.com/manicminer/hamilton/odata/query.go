package odata

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type ConsistencyLevel string

const (
	ConsistencyLevelEventual ConsistencyLevel = "eventual"
)

type Metadata string

const (
	MetadataFull    Metadata = "full"
	MetadataMinimal Metadata = "minimal"
	MetadataNone    Metadata = "none"
)

type Query struct {
	// ConsistencyLevel sets the corresponding http header
	ConsistencyLevel ConsistencyLevel

	// Metadata indicates how much control information is requested (services assume "minimal" when not specified)
	Metadata Metadata

	// Count includes a count of the total number of items in a collection alongside the page of data values
	Count bool

	// Expand includes the expanded resource or collection referenced by a single relationship
	Expand Expand

	// Filter retrieves just a subset of a collection, or relationships like members, memberOf, transitiveMembers, and transitiveMemberOf
	Filter string

	// Format specifies the media format of the items returned
	Format Format

	// OrderBy specify the sort order of the items returned
	OrderBy OrderBy

	// Search restricts the results of a request to match a search criterion
	Search string // complicated

	// Select returns a set of properties that are different than the default set for an individual resource or a collection of resources
	Select []string

	// Skip sets the number of items to skip at the start of a collection
	Skip int

	// Top specifies the page size of the result set
	Top int
}

// Headers returns an http.Header map containing OData specific headers, for use in requests
func (q Query) Headers() http.Header {
	// Take extra care over canonicalization of header names
	headers := http.Header{
		"Odata-Maxversion": []string{ODataVersion},
		"Odata-Version":    []string{ODataVersion},
	}

	accept := "application/json; charset=utf-8; IEEE754Compatible=false"
	if q.Metadata != "" {
		accept = fmt.Sprintf("%s; odata.metadata=%s", accept, q.Metadata)
	}
	headers.Set("Accept", accept)

	if q.ConsistencyLevel != "" {
		headers.Set("Consistencylevel", string(q.ConsistencyLevel))
	}

	return headers
}

// AppendHeaders returns the provided http.Header map with OData specific headers appended, for use in requests
func (q Query) AppendHeaders(header http.Header) http.Header {
	if header == nil {
		header = http.Header{}
	}
	for k, v := range q.Headers() {
		if len(v) > 0 {
			header.Set(k, v[0])
		}
	}
	return header
}

// Values returns a url.Values map containing OData specific query parameters, for use in requests
func (q Query) Values() url.Values {
	p := url.Values{}
	if q.Count {
		p.Add("$count", fmt.Sprintf("%t", q.Count))
	}
	if expand := q.Expand.String(); expand != "" {
		p.Add("$expand", expand)
	}
	if q.Filter != "" {
		p.Add("$filter", q.Filter)
	}
	if format := string(q.Format); format != "" {
		p.Add("$format", format)
	}
	if orderBy := q.OrderBy.String(); orderBy != "" {
		p.Add("$orderby", orderBy)
	}
	if q.Search != "" {
		p.Add("$search", fmt.Sprintf(`"%s"`, q.Search))
	}
	if len(q.Select) > 0 {
		p.Add("$select", strings.Join(q.Select, ","))
	}
	if q.Skip > 0 {
		p.Add("$skip", strconv.Itoa(q.Skip))
	}
	if q.Top > 0 {
		p.Add("$top", strconv.Itoa(q.Top))
	}
	return p
}

// AppendValues returns the provided url.Values map with OData specific query parameters appended, for use in requests
func (q Query) AppendValues(values url.Values) url.Values {
	if values == nil {
		values = url.Values{}
	}
	for k, v := range q.Values() {
		if len(v) > 0 {
			values.Set(k, v[0])
		}
	}
	return values
}

type Expand struct {
	Relationship string
	Select       []string
}

func (e Expand) String() (val string) {
	val = e.Relationship
	if len(e.Select) > 0 {
		val = fmt.Sprintf("%s($select=%s)", val, strings.Join(e.Select, ","))
	}
	return
}

type Format string

const (
	FormatJson Format = "json"
	FormatAtom Format = "atom"
	FormatXml  Format = "xml"
)

type Direction string

const (
	Ascending  Direction = "asc"
	Descending Direction = "desc"
)

type OrderBy struct {
	Field     string
	Direction Direction
}

func (o OrderBy) String() (val string) {
	val = o.Field
	if val != "" && o.Direction != "" {
		val = fmt.Sprintf("%s %s", val, o.Direction)
	}
	return
}
