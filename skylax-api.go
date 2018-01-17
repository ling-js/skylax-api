package main

import (
	"fmt"
	"github.com/julienschmidt/httprouter"
	"github.com/rs/cors"
	"log"
	"net/http"
	"time"
)

// Init global Variables
var Verbose = true

func main() {
	router := httprouter.New()
	router.HandlerFunc("GET", "/search", SearchHandler)
	router.HandlerFunc("POST", "/generate", GenerateHandler)
	router.HandlerFunc("GET", "/value", LookupHandler)

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


// Tracks the time elapsed since start.
func Timetrack(start time.Time, name string) {
	elapsed := time.Since(start)
	fmt.Printf("%s finished in %s\n", name, elapsed)
}
