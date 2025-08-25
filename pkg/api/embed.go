package api

import (
	"embed"
)

//go:generate go tool oapi-codegen -config cfg.yaml api.yaml

//go:embed index.html
//go:embed api.yaml
//go:embed web-components.min.js
//go:embed styles.min.css
var SwaggerUI embed.FS
