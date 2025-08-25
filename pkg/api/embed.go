package api

import (
	"embed"
)

//go:embed index.html
//go:embed api.yaml
//go:embed web-components.min.js
//go:embed styles.min.css
var SwaggerUI embed.FS
