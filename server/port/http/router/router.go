package router

import "net/http"

// Route is a single method-aware mux registration.
type Route struct {
	Pattern string
	Handler http.HandlerFunc
}

// Register attaches each route to the mux.
func Register(mux *http.ServeMux, routes []Route) {
	for _, route := range routes {
		mux.HandleFunc(route.Pattern, route.Handler)
	}
}
