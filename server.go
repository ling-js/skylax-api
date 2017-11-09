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
	//router.GET("/generate", GenerateHandler)
	//router.GET("/data", DataHandler)

	log.Fatal(http.ListenAndServe(":1337", router))
}
