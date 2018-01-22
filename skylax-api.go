package main

import (
	"flag"
	"fmt"
	"github.com/julienschmidt/httprouter"
	"github.com/rs/cors"
	"log"
	"net/http"
	"time"
)

// Init global Variables
var Verbose = false

func main() {

	// Get command-line flags
	verbose := flag.Bool("v", false, "toggle verbose output")
	flag.Parse()
	if *verbose {
			Verbose = true
	}

	// Create Routes
	router := httprouter.New()
	router.HandlerFunc("GET", "/search", SearchHandler)
	router.HandlerFunc("POST", "/generate", GenerateHandler)
	router.HandlerFunc("GET", "/value", LookupHandler)

	// Set CORS Headers
	handler := cors.Default().Handler(router)
	c := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "OPTIONS"},
	})

	// Serve gernerated Images
	router.ServeFiles("/data/*filepath", http.Dir("data/"))

	// Start Server.
	log.Fatal(http.ListenAndServe(":8080", c.Handler(handler)))
}

// Tracks the time elapsed since start.
func Timetrack(start time.Time, name string) {
	elapsed := time.Since(start)
	fmt.Printf("%s finished in %s\n", name, elapsed)
}