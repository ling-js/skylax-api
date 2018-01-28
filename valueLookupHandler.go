package main

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net/http"
	"os/exec"
	"time"
)

// Response Schemata of gdallocation script
type report struct {
	Bands []bandreport `xml:"BandReport"`
}

type bandreport struct {
	File  string `xml:"LocationInfo>File"`
	Value string `xml:"Value"`
}

// LookupHandler handles all Requests for concrete Dataset values
func LookupHandler(w http.ResponseWriter, r *http.Request) {
	defer Timetrack(time.Now(), "ValueLookup")
	// Get Query Parameters
	q := r.URL.Query()
	xcoord := q.Get("x")
	ycoord := q.Get("y")
	datasetname := q.Get("d")
	bandname := q.Get("b")

	// If Verbose is toggle print parsed output
	if Verbose {
		fmt.Print("Request to /value with parameters: ")
		fmt.Println(q)
	}

	// local variables
	var output []byte
	var err error
	tci := false

	// Check if S2A Dataset
	if datasetname[:8] != "SENTINEL" {

		// check if TCI Dataset
		if bandname[len(bandname)-11:len(bandname)-8] == "TCI" {
			tci = true
		}

		// Get Name of dynamically named subfolder
		datalocation := DataSource + datasetname + "/GRANULE/"
		subfolder, err := ioutil.ReadDir(datalocation)
		if err != nil {
			w.WriteHeader(404)
			w.Write([]byte("Cannot find Dataset"))
			return
		}

		// Get Resolution
		resolution := bandname[len(bandname)-7 : len(bandname)-5]

		// Get Pixel Data
		datasetlocation := DataSource + datasetname + "/GRANULE/" + subfolder[0].Name() + "/IMG_DATA/R" + resolution + "m/" + bandname
		output, err = exec.Command("gdallocationinfo", "-xml", "-wgs84", datasetlocation, xcoord, ycoord).Output()
		if err != nil {
			fmt.Println(err.Error())
		}
	} else {
		// check if TCI Dataset
		if bandname == "TCI" {
			tci = true
		}
		// Get Pixel Data
		output, err = exec.Command("gdallocationinfo", "-xml", "-wgs84", datasetname, xcoord, ycoord).Output()
	}
	// Check for error while executing gdallocationinfo
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte("Error executing Value Lookup. Error was: " + err.Error()))
	}

	// Parse Output from xml to Go struct
	var v report
	err = xml.Unmarshal(output, &v)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte("Error parsing xml. Error was: " + err.Error()))
	}

	// Compute TCI Values into json array
	if tci {
		w.Write([]byte("[" + v.Bands[0].Value + "," + v.Bands[1].Value + "," + v.Bands[2].Value + "]"))
		return
	}

	// Check if S2A Dataset
	if datasetname[:8] == "SENTINEL" {
		// Get and Return Value corresponding to provided bandname
		for index := range v.Bands {
			fmt.Println("")
			// Extract Bandname from Filename
			band := v.Bands[index].File
			band = band[len(band)-7 : len(band)-4]
			if bandname == band {
				w.Write([]byte(v.Bands[index].Value))
				return
			}
		}
	} else {
		if v.Bands != nil {
			w.Write([]byte(v.Bands[0].Value))
			return
		}
	}

	// If Band is not found in Dataset
	w.WriteHeader(404)
	w.Write(append([]byte("Error while finding Band in Dataset \n Debug output below: \n \n "), output...))
}
