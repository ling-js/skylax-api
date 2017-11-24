package main

import (
	"github.com/julienschmidt/httprouter"
	"github.com/segmentio/ksuid"
	"log"
	"net/http"
)

// Init global Variables
var Ksuid = ksuid.New()

func main() {
	router := httprouter.New()
	router.GET("/search", SearchHandler)
	router.POST("/generate", GenerateHandler)

	// Serve gernerated Images
	router.ServeFiles("/data/*filepath", http.Dir("data/"))

	// Start Server and crash when it fails.
	log.Fatal(http.ListenAndServe(":8080", router))
}
