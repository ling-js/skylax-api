package main

import (
	"github.com/julienschmidt/httprouter"
	"log"
	"net/http"
)

//TODO(specki)
func main() {
	router := httprouter.New()
	router.GET("/search", SearchHandler)
	router.POST("/generate", GenerateHandler)
	log.Fatal(http.ListenAndServe(":8081", router))
}
