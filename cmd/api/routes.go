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
	router.HandlerFunc(
		http.MethodGet,
		"/v1/targets",
		app.requireActivatedUser(app.listTargetsHandler),
	)
	router.HandlerFunc(
		http.MethodPost,
		"/v1/targets",
		app.requireActivatedUser(app.createTargetHandler),
	)
	router.HandlerFunc(
		http.MethodGet,
		"/v1/targets/:uuid",
		app.requireActivatedUser(app.showTargetHandler),
	)
	router.HandlerFunc(
		http.MethodPatch,
		"/v1/targets/:uuid",
		app.requireActivatedUser(app.updateTargetHandler),
	)
	router.HandlerFunc(
		http.MethodDelete,
		"/v1/targets/:uuid",
		app.requireActivatedUser(app.deleteTargetHandler),
	)
	router.HandlerFunc(
		http.MethodGet,
		"/v1/targets/:uuid/activities",
		app.requireActivatedUser(app.listTargetActivitiesHandler),
	)

	// Activities routes
	router.HandlerFunc(
		http.MethodGet,
		"/v1/activities",
		app.requireActivatedUser(app.listActivitiesHandler),
	)
	router.HandlerFunc(
		http.MethodPost,
		"/v1/activities",
		app.requireActivatedUser(app.createActivityHandler),
	)
	router.HandlerFunc(
		http.MethodGet,
		"/v1/activities/:uuid",
		app.requireActivatedUser(app.showActivityHandler),
	)
	router.HandlerFunc(
		http.MethodPatch,
		"/v1/activities/:uuid",
		app.requireActivatedUser(app.updateActivityHandler),
	)
	router.HandlerFunc(
		http.MethodDelete,
		"/v1/activities/:uuid",
		app.requireActivatedUser(app.deleteActivityHandler),
	)

	// Users routes
	router.HandlerFunc(
		http.MethodGet,
		"/v1/users/me",
		app.requireActivatedUser(app.showCurrentUserHandler),
	)
	router.HandlerFunc(http.MethodPost, "/v1/users", app.registerUserHandler)
	// Activate a user account
	router.HandlerFunc(http.MethodPut, "/v1/users/activated", app.activateUserHandler)
	router.HandlerFunc(http.MethodPut, "/v1/users/password", app.updateUserPasswordHandler)

	// Tokens routes
	// Generate a new activation token for a user
	router.HandlerFunc(http.MethodPost, "/v1/tokens/activation", app.createActivationTokenHandler)
	// Generate a new authentication token (access token & refresh token) for a user
	router.HandlerFunc(
		http.MethodPost,
		"/v1/tokens/authentication",
		app.createAuthenticationTokenHandler,
	)
	// Generate a new pair of authentication tokens for a user by using a valid refresh token
	router.HandlerFunc(http.MethodPost, "/v1/tokens/refresh", app.refreshAuthenticationTokenHandler)
	router.HandlerFunc(
		http.MethodPost,
		"/v1/tokens/password-reset",
		app.createPasswordResetTokenHandler,
	)

	// Sessions routes
	router.HandlerFunc(
		http.MethodDelete,
		"/v1/sessions/:uuid",
		app.requireAuthenticatedUser(app.deleteSessionHandler),
	)

	return app.recoverPanic(app.rateLimit(app.authenticate(router)))
}
