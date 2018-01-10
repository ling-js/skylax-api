package main

import (
	"github.com/julienschmidt/httprouter"
	"github.com/rs/cors"
	"github.com/segmentio/ksuid"
	"log"
	"net/http"
)

// Init global Variables
var Ksuid = ksuid.New()
var Verbose = true
func main() {
	router := httprouter.New()
	router.HandlerFunc("GET", "/search", SearchHandler)
	router.HandlerFunc("POST", "/generate", GenerateHandler)

	handler := cors.Default().Handler(router)
	c := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "OPTIONS"},
	})

	// Serve gernerated Images
	router.ServeFiles("/data/*filepath", http.Dir("data/"))

	// Start Server and crash when it fails.
	log.Fatal(http.ListenAndServe(":8080", c.Handler(handler)))
}
