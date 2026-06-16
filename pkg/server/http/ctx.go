package http

import "net/http"

type Ctx struct {
	statusCode int
	rw         http.ResponseWriter
	req        *http.Request

	// skipAutoResponse is explicitly set by the application layer to signal
	// that it has taken full ownership of the HTTP response lifecycle (e.g.,
	// manually writing a raw YAML file, streaming binaries, or custom error handling).
	//
	// This defaults to false, meaning existing code need not do anything to maintain
	// current behavior.
	//
	// Note: The boundary is absolute with no complex fallback mechanisms. The app layer
	// either relies entirely on the framework, or handles everything itself. Once this
	// flag is set to true, the framework completely backs off and does nothing on both
	// successful and error execution paths, allowing the app layer to safely return
	// errors (e.g., mid-stream io.Copy failures) without triggering framework response writes.
	skipAutoResponse bool
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

// SkipAutoResponse signals to the framework that the application layer
// will handle writing the status code and body directly.
func (ctx *Ctx) SkipAutoResponse() {
	ctx.skipAutoResponse = true
}

// IsAutoResponseSkipped checks if the application layer took over the response.
func (ctx *Ctx) IsAutoResponseSkipped() bool {
	return ctx.skipAutoResponse
}
