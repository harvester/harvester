package gout

import (
	"github.com/guonaihong/gout/core"
)

// ReadCloseFail required for internal testing, external can be ignored
type ReadCloseFail = core.ReadCloseFail

// H is short for map[string]interface{}
type H = core.H

// A variable is short for []interface{}
type A = core.A

// FormFile encounter the FormFile variable in the form encoder will open the file from the file
type FormFile = core.FormFile

// FormMem when encountering the FormMem variable in the form encoder, it will be read from memory
type FormMem = core.FormMem

// FormType custom form names and stream types are available
type FormType = core.FormType
