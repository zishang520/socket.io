package transports

import (
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/servers/engine/v3/log"
	"github.com/zishang520/socket.io/servers/engine/v3/types"
)

var (
	jsonp_log = log.NewLog("engine:jsonp")

	rDoubleSlashes = regexp.MustCompile(`\\\\n`)
	rSlashes       = regexp.MustCompile(`(\\)?\\n`)
	rNumber        = regexp.MustCompile(`[^0-9]`)
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
	if data, err := url.ParseQuery(data.String()); err != nil {
		j.OnError("jsonp read error", err)
	} else {
		if data.Has("d") {
			_data := rSlashes.ReplaceAllStringFunc(data.Get("d"), func(m string) string {
				if parts := rSlashes.FindStringSubmatch(m); parts[1] != "" {
					return parts[0]
				}
				return "\n"
			})
			// client will send already escaped newlines as \\\\n and newlines as \\n
			// \\n must be replaced with \n and \\\\n with \\n
			j.Polling.OnData(types.NewStringBufferString(rDoubleSlashes.ReplaceAllString(_data, "\\n")))
		}
	}
}

func (j *jsonp) DoWrite(ctx *types.HttpContext, data types.BufferInterface, options *packet.Options, callback func(error)) {
	// prepare response
	res := types.NewStringBufferString(j.head)
	encoder := json.NewEncoder(res)
	// we must output valid javascript, not valid json
	// see: http://timelessrepo.com/json-isnt-a-javascript-subset
	//
	if err := encoder.Encode(data.String()); err != nil {
		ctx.Cleanup()
		defer callback(err)

		ctx.SetStatusCode(http.StatusInternalServerError)
		ctx.Write(nil)
		return
	}
	// Since 1.18 the following source code is very annoying '\n' bytes
	res.Truncate(res.Len() - 1) // '\n' 😑
	res.WriteString(j.foot)
	j.Polling.DoWrite(ctx, res, options, callback)
}
