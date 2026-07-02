package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func doReq(e *echo.Echo, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestEnvelope_WrapsSuccessJSON(t *testing.T) {
	e := echo.New()
	g := e.Group("/v2", EnvelopeMiddleware)
	g.GET("/ping", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]any{"x": 1})
	})

	rec := doReq(e, http.MethodGet, "/v2/ping")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var env struct {
		Data  map[string]any `json:"data"`
		Error *struct {
			Message string `json:"message"`
			Code    int    `json:"code"`
		} `json:"error"`
		Meta map[string]any `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("body not enveloped JSON: %v / %s", err, rec.Body.String())
	}
	if env.Error != nil {
		t.Errorf("error should be null on success, got %+v", env.Error)
	}
	if env.Data["x"] != float64(1) {
		t.Errorf("data.x = %v, want 1", env.Data["x"])
	}
	if env.Meta["version"] != "v2" {
		t.Errorf("meta.version = %v, want v2", env.Meta["version"])
	}
}

func TestEnvelope_WrapsErrorJSON(t *testing.T) {
	e := echo.New()
	g := e.Group("/v2", EnvelopeMiddleware)
	g.GET("/boom", func(c echo.Context) error {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad input"})
	})

	rec := doReq(e, http.MethodGet, "/v2/boom")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	var env struct {
		Data  json.RawMessage `json:"data"`
		Error *struct {
			Message string `json:"message"`
			Code    int    `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if env.Error == nil || env.Error.Message != "bad input" || env.Error.Code != 400 {
		t.Errorf("error = %+v, want {message:'bad input', code:400}", env.Error)
	}
	if string(env.Data) != "null" {
		t.Errorf("data = %s, want null", env.Data)
	}
}

func TestEnvelope_PassesThroughNonJSON(t *testing.T) {
	e := echo.New()
	g := e.Group("/v2", EnvelopeMiddleware)
	payload := []byte("PK\x03\x04 binary xlsx")
	g.GET("/file", func(c echo.Context) error {
		return c.Blob(http.StatusOK, "application/octet-stream", payload)
	})

	rec := doReq(e, http.MethodGet, "/v2/file")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != string(payload) {
		t.Errorf("binary body altered: got %q", rec.Body.String())
	}
}
