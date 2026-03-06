package http

import (
	_ "embed"
	"net/http"
)

//go:embed swagger.json
var swaggerJSON []byte

func swaggerHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(swaggerJSON)
	})
}
