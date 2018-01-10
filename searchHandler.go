package main

import (
	"encoding/json"
	"github.com/ling-js/go-gdal"
	"github.com/paulsmith/gogeos/geos"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"fmt"
)

func SearchHandler(w http.ResponseWriter, r *http.Request) {
	defer Timetrack(time.Now(), "Search ")
	q := r.URL.Query()
	datasets, err := ioutil.ReadDir("/opt/sentinel2")
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte("Unable to open Data Repository: " + err.Error()))
		return
	}

	bboxstring := q.Get("bbox")
	var bbox *geos.Geometry

	// Check if bbox is supplied
	if bboxstring != "" {
		coordinates := strings.Split(bboxstring, ",")

		polygon :=
			"POLYGON((" +
				coordinates[0] + " " +
				coordinates[1] + "," +
				coordinates[0] + " " +
				coordinates[3] + "," +
				coordinates[2] + " " +
				coordinates[3] + "," +
				coordinates[1] + " " +
				coordinates[2] + "," +
				coordinates[0] + " " +
				coordinates[1] + "))"

		bbox, err = geos.FromWKT(polygon)
		if err != nil {
			w.WriteHeader(400)
			if Verbose {
				fmt.Println(err)
			}
			w.Write([]byte(err.Error()))
			return
		}
	}
	// Filter by Name
	err = nameFilter(datasets, q.Get("substring"))
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte("Unable to filter by substring: " + err.Error()))
		return
	}
	// Filter by bbox, startDate, endDate
	startDate := q.Get("startdate")
	endDate := q.Get("enddate")
	filterDates := startDate != "" && endDate != ""
	filterBox := bbox != nil

	// Only Filter if filters are supplied
	if filterDates || filterBox {
		err = metaDataFilter(datasets, q.Get("startdate"), q.Get("enddate"), bbox, filterDates, filterBox)
		if err != nil {
			w.WriteHeader(500)
			w.Write([]byte("Unable to filter by metadata: " + err.Error()))
			return
		}
	}

	// Prepare metadata output
	var metadatachannel = make(chan []string, 8)
	metadatajson := []byte("[")

	// Get page from URL
	pagestring := q.Get("page")
	page := 0
	if pagestring != "" {
		page, err = strconv.Atoi(pagestring)
		if err != nil {
			w.WriteHeader(400)
			w.Write([]byte("Unable to get page parameter from URL: " + err.Error()))
			return
		}
	}

	// Get metadata
	metadatacounter, totalcounter, err := getMetaData(datasets, page, metadatachannel)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte("Unable to retrieve Metadata " + err.Error()))
		return
	}

	// Write max page into response Headers
	w.Header().Set("X-Dataset-Count", strconv.Itoa(totalcounter))
	w.Header().Set("Access-Control-Expose-Headers", "X-Dataset-Count")

	// Edge Case where length of returned Array is 0
	if metadatacounter == 0 {
		metadatajson = append(metadatajson, []byte(",")...)
	}

	// Convert String slice to json
	for i := 0; i < metadatacounter; i++ {
		fields := make(map[string]string)
		a := <-metadatachannel
		for index := range a {
			keyValuePair := strings.Split(a[index], "=")
			fields[keyValuePair[0]] = keyValuePair[1]
		}
		jsonstring, err := json.Marshal(fields)
		if err != nil {
			w.WriteHeader(500)
			w.Write([]byte(err.Error()))
			return
		}
		metadatajson = append(metadatajson, jsonstring...)
		metadatajson = append(metadatajson, []byte(",")...)
	}

	// replace last commata with ] to create valid json Array
	metadatajson[len(metadatajson)-1] = byte(']')

	// Write Response with default 200 OK Status Code
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write(metadatajson)
}

// nameFilter sets all Elements in datasets to nil when string does not match
func nameFilter(datasets []os.FileInfo, name string) error {
	for index := range datasets {
		match, err := regexp.MatchString(name, datasets[index].Name())
		if err != nil {
			return err
		}
		if !match {
			datasets[index] = nil
		}
	}
	return nil
}

// metaDataFilter sets all Elements in datasets to nil when generationTime is not withing bounds set by startDate and endDate or does not intersect bbox.
func metaDataFilter(datasets []os.FileInfo, startDateRAW, endDateRAW string, bbox *geos.Geometry, filterDates, filterBox bool) error {
	var startDate, endDate time.Time
	if filterDates {
		var err, err2 error
		startDate, err = time.Parse(time.RFC3339, startDateRAW)
		endDate, err2 = time.Parse(time.RFC3339, endDateRAW)
		if err != nil {
			fmt.Println(err)
			return err
		}
		if err2 != nil {
			return err2
			fmt.Println(err2)
		}
	}
	for index := range datasets {
		dataset, err := gdal.Open("/opt/sentinel2/"+datasets[index].Name()+"/MTD_MSIL1C.xml", gdal.ReadOnly)
		if err != nil {
			//TODO(specki): Temporaeres workaround fuer 2A datasets
			datasets[index] = nil
			return nil
			// return err
		}
		// Get Metadata
		generationTimeRAW := dataset.Metadata("")[12][16:]
		footprintRAW := dataset.Metadata("")[9][10:]

		if filterDates {
			// Conversion to usable Time
			generationTime, err := time.Parse(time.RFC3339, generationTimeRAW)
			if err != nil {
				return err
			}
			// Check if Dataset Generation Time is between specified Dates
			if generationTime.Before(startDate) || generationTime.After(endDate) {
				datasets[index] = nil
			}

		}
		if filterBox {
			// Convert to usable Geometry
			footprint, err := geos.FromWKT(footprintRAW)
			if err != nil {
				return err
			}

			// Check if Dataset overlaps with bbox
			intersects, err := footprint.Intersects(bbox)
			if err != nil {
				return err
			}
			// Check if Dataset Generation Time is between specified Dates
			if !intersects {
				datasets[index] = nil
			}
		}
		// Close Dataset
		dataset.Close()
	}
	return nil
}

// Gets the metaData for cap(metadata) items starting with element page*cap(metadata).
func getMetaData(datasets []os.FileInfo, page int, metadata chan []string) (metadatacount, totalcounter int, error error) {
	// Total counts of elements found in datasets
	totalcounter = 0
	// Number of Elements pushed into channel
	metadatacounter := 0
	pagesize := cap(metadata)

	for index := range datasets {
		// Fetch Metadata if Dataset is not sorted out + is on correct page
		if datasets[index] != nil {
			// Shortcircuit invalidates totalcounter needed for pagination
			//if metadatacounter == pagesize {
				// Pushed all to channel
			//	return metadatacounter, totalcounter, nil
			//}
			totalcounter++
			// Push to metadata if on correct page and not already full
			if totalcounter > page*pagesize && metadatacounter < pagesize {
				metadatacounter++
				dataset, err := gdal.Open(
					"/opt/sentinel2/"+datasets[index].Name()+"/MTD_MSIL1C.xml",
					gdal.ReadOnly)
				dataset2, err2 := gdal.Open(
					"/opt/sentinel2/"+datasets[index].Name()+"/MTD_MSIL2A.xml",
					gdal.ReadOnly)
				if err == nil {
					metadata <- append(dataset.Metadata(""), dataset.Metadata("Subdatasets")...)
					dataset.Close()
				} else if err2 == nil {
					// metadata <- append(dataset2.Metadata(""), dataset2.Metadata("Subdatasets")...)
					metadata <- append([]string {"CLOUD_COVERAGE_ASSESSMENT=S2A IS NOT SUPPORTED YET"})
					dataset2.Close()
				} else {
					return metadatacounter, 0, err
				}
			}
		}
	}
	return metadatacounter, totalcounter, nil
}
