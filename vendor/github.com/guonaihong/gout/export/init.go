package export

import (
	"github.com/guonaihong/gout/dataflow"
)

var (
	defaultCurl = Curl{}
)

func init() {
	dataflow.Register("curl", &defaultCurl)
}
