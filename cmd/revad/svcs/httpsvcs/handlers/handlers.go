package handlers

import (
	"net/http"
)

// Handlers is a map of all registered handlers to be used from http services.
var Handlers = map[string]HandlerChain{}

// HandlerChain is the the type that http handlers need to register.
type HandlerChain func(http.Handler) http.Handler

// Register register a handler chain.
func Register(name string, chain HandlerChain) {
	Handlers[name] = chain
}
