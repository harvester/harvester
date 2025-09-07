package dataflow

import (
	"path"
	"strings"
)

const (
	httpProto  = "http://"
	httpsProto = "https://"
)

func cleanPaths(rv string) string {
	if rv == "" {
		return rv
	}

	if strings.HasPrefix(rv, httpProto) || strings.HasPrefix(rv, httpsProto) {
		var proto, domainPath, query string
		protoEndIdx := strings.Index(rv, "//") + 2
		queryIdx := strings.Index(rv, "?")

		proto = rv[:protoEndIdx]
		if queryIdx != -1 {
			domainPath = rv[protoEndIdx:queryIdx]
			query = rv[queryIdx:]
		} else if rv != proto {
			domainPath = rv[protoEndIdx:]
		}
		if domainPath != "" {
			appendSlash := len(domainPath) > 0 && domainPath[len(domainPath)-1] == '/'
			domainPath = path.Clean(domainPath)
			if appendSlash {
				domainPath += "/"
			}
		}
		return proto + domainPath + query
	}

	appendSlash := len(rv) > 0 && rv[len(rv)-1] == '/'
	cleanPath := path.Clean(rv)
	if appendSlash {
		return cleanPath + "/"
	}

	return cleanPath
}
