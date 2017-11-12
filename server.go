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
	log.Fatal(http.ListenAndServe(":1337", router))
}
