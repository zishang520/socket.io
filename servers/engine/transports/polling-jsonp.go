// Package transports implements the JSONP polling transport for Engine.IO.
package transports

import (
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

var (
	rNumber = regexp.MustCompile(`[^0-9]`)

	// OPTIMIZACIÓN: Usar strings.Replacer en lugar de regex.
	// Es mucho más rápido para reemplazos de cadenas de texto fijas.
	// Reemplaza "\\n" por "\n" y luego "\n" (que originalmente era \\n) por un salto de línea real.
	// El orden de los pares es importante: el reemplazo más largo primero.
	jsonpReplacer = strings.NewReplacer(`\\\\n`, `\n`, `\\n`, "\n")
)

type jsonp struct {
	Polling

	head string
	foot string
}

// JSON-P polling transport.
func MakeJSONP() Jsonp {
	j := &jsonp{Polling: MakePolling()}

	j.Prototype(j)

	return j
}

func NewJSONP(ctx *types.HttpContext) Jsonp {
	j := MakeJSONP()

	j.Construct(ctx)

	return j
}

func (j *jsonp) Construct(ctx *types.HttpContext) {
	j.Polling.Construct(ctx)

	j.head = "___eio[" + rNumber.ReplaceAllString(ctx.Query().Peek("j"), "") + "]("
	j.foot = ");"
}

func (j *jsonp) OnData(data types.BufferInterface) {
	payload, err := url.ParseQuery(data.String())
	if err != nil {
		j.OnError("jsonp read error", err)
		return
	}

	if payload.Has("d") {
		// client will send already escaped newlines as \\\\n and newlines as \\n
		// \\n must be replaced with \n and \\\\n with \\n
		j.Polling.OnData(types.NewStringBufferString(jsonpReplacer.Replace(payload.Get("d"))))
	}
}

func (j *jsonp) DoWrite(ctx *types.HttpContext, data types.BufferInterface, options *packet.Options, callback func(error)) {
	// Note: We must output valid JavaScript, not just JSON.
	// JSON is not a strict subset of JavaScript, so directly writing JSON
	// may result in runtime errors in JS.
	// See: https://timelessrepo.com/json-isnt-a-javascript-subset
	payload, err := json.Marshal(data.String())
	if err != nil {
		ctx.Cleanup()
		defer callback(err)

		// Respond with 500 Internal Server Error if encoding fails
		ctx.SetStatusCode(http.StatusInternalServerError)
		ctx.Write(nil)
		return
	}

	// prepare response
	res := types.NewStringBufferString(j.head)
	res.Write(payload)
	res.WriteString(j.foot)
	j.Polling.DoWrite(ctx, res, options, callback)
}
