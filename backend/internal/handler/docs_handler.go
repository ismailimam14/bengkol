package handler

import (
	"net/http"

	"github.com/bengkol/backend/docs"
)

// DocsHandler serves Swagger UI, ReDoc, and the embedded OpenAPI specification.
type DocsHandler struct{}

// NewDocsHandler creates a new DocsHandler.
func NewDocsHandler() *DocsHandler {
	return &DocsHandler{}
}

// OpenAPI serves the raw OpenAPI 3.0 YAML specification.
func (h *DocsHandler) OpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(docs.OpenAPIYAML)
}

// SwaggerUI serves the interactive Swagger UI web interface.
func (h *DocsHandler) SwaggerUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	html := `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <meta name="description" content="Bengkol Workshop & Repair-Shop Management API Documentation" />
  <title>Bengkol API - Swagger Documentation</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css" />
  <link rel="icon" type="image/png" href="https://unpkg.com/swagger-ui-dist@5/favicon-32x32.png" sizes="32x32" />
  <style>
    body {
      margin: 0;
      padding: 0;
      background: #fafafa;
    }
    .topbar {
      display: none !important;
    }
    .swagger-ui .info {
      margin: 30px 0;
    }
  </style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js" crossorigin></script>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-standalone-preset.js" crossorigin></script>
  <script>
    window.onload = () => {
      window.ui = SwaggerUIBundle({
        url: '/docs/openapi.yaml',
        dom_id: '#swagger-ui',
        deepLinking: true,
        presets: [
          SwaggerUIBundle.presets.apis,
          SwaggerUIStandalonePreset
        ],
        layout: "StandaloneLayout",
        persistAuthorization: true,
        displayRequestDuration: true,
        filter: true,
        tryItOutEnabled: true,
        defaultModelsExpandDepth: 2,
        defaultModelExpandDepth: 2,
        docExpansion: "list"
      });
    };
  </script>
</body>
</html>`
	_, _ = w.Write([]byte(html))
}

// ReDoc serves the modern ReDoc documentation web interface.
func (h *DocsHandler) ReDoc(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	html := `<!DOCTYPE html>
<html>
  <head>
    <title>Bengkol API - ReDoc Documentation</title>
    <meta charset="utf-8"/>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <link href="https://fonts.googleapis.com/css?family=Montserrat:300,400,700|Roboto:300,400,700" rel="stylesheet">
    <style>
      body {
        margin: 0;
        padding: 0;
      }
    </style>
  </head>
  <body>
    <redoc spec-url='/docs/openapi.yaml'></redoc>
    <script src="https://cdn.redoc.ly/redoc/latest/bundles/redoc.standalone.js"> </script>
  </body>
</html>`
	_, _ = w.Write([]byte(html))
}
