package main

import (
	"github.com/julienschmidt/httprouter"
	"log"
	"net/http"
	"time"
)

//TODO(specki)
func main() {
	router := httprouter.New()
	router.GET("/search", SearchHandler)
	//router.GET("/generate", GenerateHandler)
	//router.GET("/data", DataHandler)
	//main3()
	log.Fatal(http.ListenAndServe(":1337", router))
}

func SearchHandlerTest(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	defer Timetrack(time.Now(), "SearchHandlerTEST")
	for i := 0; i < 500; i++ {
		SearchHandler(w,r,nil)
	}
}