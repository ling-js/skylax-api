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

// Verbose command line Parameter
var Verbose = false

// DataSource command line Parameter
var DataSource = ""

func main() {

	// Get command-line flags
	filelocation := flag.String("src", "/opt/sentinel2/", "set source directory for datasets")
	verbose := flag.Bool("v", false, "toggle verbose output")
	flag.Parse()
	if *verbose {
		Verbose = true
	}
	if *filelocation != "" {
		DataSource = *filelocation
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

// Timetrack racks the time elapsed since start. and logs it to console output
func Timetrack(start time.Time, name string) {
	elapsed := time.Since(start)
	fmt.Printf("%s finished in %s\n", name, elapsed)
}
