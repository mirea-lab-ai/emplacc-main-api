package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// Router is the subset of *echo.Echo / *echo.Group used by the Register* functions.
// Both types satisfy it, so every route group can be registered twice: once raw on
// the root (v1) and once under the /v2 group that applies EnvelopeMiddleware.
type Router interface {
	GET(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	POST(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	PUT(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	DELETE(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	PATCH(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	Group(prefix string, m ...echo.MiddlewareFunc) *echo.Group
}

// v2Envelope is the normalized response shape under /v2: {data, error, meta}.
type v2Envelope struct {
	Data  json.RawMessage `json:"data"`
	Error *v2Error        `json:"error"`
	Meta  map[string]any  `json:"meta"`
}

type v2Error struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// captureWriter buffers the handler's output instead of sending it, so the
// middleware can re-wrap it. It embeds the real writer only for Header() access;
// Write/WriteHeader are intercepted.
type captureWriter struct {
	http.ResponseWriter
	buf    bytes.Buffer
	status int
}

func (w *captureWriter) WriteHeader(code int) { w.status = code }
func (w *captureWriter) Write(b []byte) (int, error) {
	return w.buf.Write(b)
}

// EnvelopeMiddleware wraps JSON responses in the v2 {data,error,meta} envelope.
// Non-JSON responses (file downloads, SSE) pass through unchanged.
func EnvelopeMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		res := c.Response()
		orig := res.Writer
		cw := &captureWriter{ResponseWriter: orig, status: http.StatusOK}
		res.Writer = cw

		err := next(c)
		res.Writer = orig
		if err != nil {
			// Let echo's error handler render to the real writer (un-enveloped).
			return err
		}

		status := res.Status
		if status == 0 {
			status = cw.status
		}
		body := bytes.TrimSpace(cw.buf.Bytes())
		contentType := orig.Header().Get(echo.HeaderContentType)

		// Only envelope JSON; stream binaries/files through untouched.
		if !strings.Contains(contentType, echo.MIMEApplicationJSON) {
			orig.WriteHeader(status)
			_, _ = orig.Write(cw.buf.Bytes())
			return nil
		}

		out, _ := json.Marshal(buildEnvelope(status, body))
		h := orig.Header()
		h.Set(echo.HeaderContentType, echo.MIMEApplicationJSONCharsetUTF8)
		h.Del("Content-Length") // length changed after wrapping
		orig.WriteHeader(status)
		_, _ = orig.Write(out)
		return nil
	}
}

func buildEnvelope(status int, body []byte) v2Envelope {
	env := v2Envelope{Meta: map[string]any{"version": "v2"}}
	if status >= 400 {
		env.Data = json.RawMessage("null")
		env.Error = &v2Error{Message: extractMessage(body), Code: status}
		return env
	}
	if len(body) == 0 {
		env.Data = json.RawMessage("null")
	} else {
		env.Data = json.RawMessage(body)
	}
	return env
}

// extractMessage pulls a human message out of the legacy error bodies
// ({"error": "..."} or {"message": "..."}), falling back to the raw text.
func extractMessage(body []byte) string {
	var m map[string]any
	if json.Unmarshal(body, &m) == nil {
		for _, k := range []string{"error", "message"} {
			if v, ok := m[k].(string); ok && v != "" {
				return v
			}
		}
	}
	return strings.TrimSpace(string(body))
}
