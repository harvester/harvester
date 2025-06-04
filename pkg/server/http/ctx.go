package http

import "net/http"

type Ctx struct {
	statusCode int
	rw         http.ResponseWriter
	req        *http.Request
}

func newDefaultHarvesterServerCtx(rw http.ResponseWriter, req *http.Request) *Ctx {
	return &Ctx{
		statusCode: http.StatusNoContent, // default is 204
		rw:         rw,
		req:        req,
	}
}

func (ctx *Ctx) SetStatus(statusCode int) { ctx.statusCode = statusCode }

// SetStatusOK sets the status code to 200.
// Some APIs return status code 200 when there is even no response body.
// So, we open this function for those APIs to invoke.
func (ctx *Ctx) SetStatusOK() { ctx.statusCode = http.StatusOK }

func (ctx *Ctx) StatusCode() int    { return ctx.statusCode }
func (ctx *Ctx) Req() *http.Request { return ctx.req }
func (ctx *Ctx) RespWriter() http.ResponseWriter {
	return ctx.rw
}
