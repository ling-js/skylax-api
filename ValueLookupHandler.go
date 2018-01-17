package main

import (
	"fmt"
	"net/http"
	"time"
)



func LookupHandler(w http.ResponseWriter, r *http.Request) {
	defer Timetrack(time.Now(), "GenerateHandler ")
	// Get Query Parameters
	q := r.URL.Query()
	xcoord := q.Get("x")
	ycoord := q.Get("y")
	datasetname := q.Get("d")
	bandname := q.Get("")


}
