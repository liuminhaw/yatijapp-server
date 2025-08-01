package main

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
)

func (app *application) routes() http.Handler {
	router := httprouter.New()

	router.NotFound = http.HandlerFunc(app.notFoundResponse)
	router.MethodNotAllowed = http.HandlerFunc(app.methodNotAllowedResponse)

	// Healthcheck
	router.HandlerFunc(http.MethodGet, "/v1/healthcheck", app.healthcheckHandler)

	// Targets routes
	router.HandlerFunc(http.MethodGet, "/v1/targets", app.listTargetsHandler)
	router.HandlerFunc(http.MethodPost, "/v1/targets", app.createTargetHandler)
	router.HandlerFunc(http.MethodGet, "/v1/targets/:uuid", app.showTargetHandler)
	router.HandlerFunc(http.MethodPatch, "/v1/targets/:uuid", app.updateTargetHandler)
	router.HandlerFunc(http.MethodDelete, "/v1/targets/:uuid", app.deleteTargetHandler)

	return app.recoverPanic(app.rateLimit(router))
}
