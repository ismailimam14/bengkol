package handler_test

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/bengkol/backend/internal/handler"
)

func TestDocsHandler_OpenAPI(t *testing.T) {
	docsHdl := handler.NewDocsHandler()
	req := httptest.NewRequest("GET", "/docs/openapi.yaml", nil)
	rec := httptest.NewRecorder()

	docsHdl.OpenAPI(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if !strings.Contains(contentType, "yaml") {
		t.Errorf("expected Content-Type to contain yaml, got %s", contentType)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "openapi: 3.0.3") {
		t.Errorf("expected body to contain 'openapi: 3.0.3'")
	}
}

func TestDocsHandler_SwaggerUI(t *testing.T) {
	docsHdl := handler.NewDocsHandler()
	req := httptest.NewRequest("GET", "/docs", nil)
	rec := httptest.NewRecorder()

	docsHdl.SwaggerUI(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "SwaggerUIBundle") {
		t.Errorf("expected body to contain SwaggerUIBundle")
	}
}

func TestDocsHandler_ReDoc(t *testing.T) {
	docsHdl := handler.NewDocsHandler()
	req := httptest.NewRequest("GET", "/redoc", nil)
	rec := httptest.NewRecorder()

	docsHdl.ReDoc(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "redoc spec-url") {
		t.Errorf("expected body to contain redoc spec-url")
	}
}

func TestDocsHandler_AllRefsExist(t *testing.T) {
	docsHdl := handler.NewDocsHandler()
	req := httptest.NewRequest("GET", "/docs/openapi.yaml", nil)
	rec := httptest.NewRecorder()
	docsHdl.OpenAPI(rec, req)

	body := rec.Body.String()
	re := regexp.MustCompile(`\$ref:\s*['"]?#/components/([^/'"\s]+)/([^/'"\s]+)['"]?`)
	matches := re.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		t.Fatal("expected to find $ref occurrences")
	}

	for _, m := range matches {
		section := m[1]
		name := m[2]

		target := "\n    " + name + ":"
		if !strings.Contains(body, target) {
			t.Errorf("Broken $ref: #/components/%s/%s does not exist in openapi.yaml", section, name)
		}
	}
}
