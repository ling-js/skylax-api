package main

import (
	"encoding/xml"
	"io/ioutil"
	"net/http"
	"os/exec"
	"time"
)

type Report struct {
	Bands []Bandreport `xml:"BandReport"`
}

type Bandreport struct {
	File  string `xml:"LocationInfo>File"`
	Value string `xml:"Value"`
}

func LookupHandler(w http.ResponseWriter, r *http.Request) {
	defer Timetrack(time.Now(), "ValueLookup")
	// Get Query Parameters
	q := r.URL.Query()
	xcoord := q.Get("x")
	ycoord := q.Get("y")
	datasetname := q.Get("d")
	bandname := q.Get("b")

	var output []byte
	var err error

	// Check if S2A Dataset
	if datasetname[:8] != "SENTINEL" {
		// Get Name of dynamically named subfolder
		datalocation := "/opt/sentinel2/" + datasetname + "/GRANULE/"
		subfolder, err := ioutil.ReadDir(datalocation)
		if err != nil {
			w.WriteHeader(404)
			w.Write([]byte("Cannot find Dataset"))
			return
		}

		// Get Resolution
		resolution := datasetname[len(datasetname)-7 : len(datasetname)-5]
		// Get Pixel Data
		datasetlocation := "/opt/sentinel2/" + datasetname + "/GRANULE/" + subfolder[0].Name() + "/IMG_DATA/R" + resolution + "m/" + bandname
		output, err = exec.Command("gdallocationinfo", "-xml", "-wgs84", datasetlocation, xcoord, ycoord).Output()
	} else {
		output, err = exec.Command("gdallocationinfo", "-xml", "-wgs84", datasetname, xcoord, ycoord).Output()
	}
	// Check for error while executing gdallocationinfo
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte("Error executing Value Lookup. Error was: " + err.Error()))
	}

	// Parse Output from xml to Go struct
	var v Report
	err = xml.Unmarshal(output, &v)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte("Error parsing xml. Error was: " + err.Error()))
	}

	// Check if S2A Dataset
	if datasetname[:8] == "SENTINEL" {
		// Get and Return Value corresponding to provided bandname
		for index := range v.Bands {
			// Extract Bandname from Filename
			band := v.Bands[index].File
			band = band[len(band)-7 : len(band)-4]
			if bandname == band {
				w.Write([]byte(v.Bands[index].Value))
				return
			}
		}
	} else {
		w.Write([]byte(v.Bands[0].Value))
		return
	}

	// If Band is not found in Dataset
	w.WriteHeader(404)
	w.Write([]byte("Cannot find Band in Dataset"))
}
