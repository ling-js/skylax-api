package main

import (
	"encoding/json"
	"github.com/julienschmidt/httprouter"
	"github.com/ling-js/go-gdal"
	"github.com/paulsmith/gogeos/geos"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func SearchHandler(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	defer Timetrack(time.Now(), "Search ")
	q := r.URL.Query()
	datasets, err := ioutil.ReadDir("/opt/sentinel2")
	if err != nil {
		log.Fatal("unable to read data directory ", err)
	}

	bboxstring := q.Get("bbox")
	var bbox *geos.Geometry

	// Check if bbox is supplied
	if bboxstring != "" {
		coordinates := strings.Split(bboxstring, ",")
		// Create geometry from bbox
		ll1, err2 := strconv.ParseFloat(coordinates[0], 64)
		ll2, err3 := strconv.ParseFloat(coordinates[1], 64)
		ur1, err4 := strconv.ParseFloat(coordinates[2], 64)
		ur2, err5 := strconv.ParseFloat(coordinates[3], 64)

		bbox, err = geos.NewPolygon([]geos.Coord{geos.NewCoord(ll1, ll2), geos.NewCoord(ur1, ur2)})
		if err != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil {
			log.Fatal("Failed to parse bounding box ", err, err2, err3, err4, err5)
			w.WriteHeader(400)
			w.Write([]byte(err.Error()))
		}
	}
	// Filter by Name
	nameFilter(datasets, q.Get("substring"))

	// Filter by bbox, startDate, endDate
	startDate := q.Get("startdate")
	endDate := q.Get("enddate")
	filterDates := startDate != "" && endDate != ""
	filterBox := bbox != nil

	// Only Filter if filters are supplied
	if filterDates || filterBox {
		metaDataFilter(datasets, q.Get("startdate"), q.Get("enddate"), bbox, filterDates, filterBox)
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
			log.Fatal("Unable to parse Page Info ", err)
			w.WriteHeader(400)
			w.Write([]byte(err.Error()))
		}
	}

	// Get metadata
	metadatacounter, err := getMetaData(datasets, page, metadatachannel)
	if err != nil {
		log.Fatal("Unable to retrieve metadata ", err)
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
	}

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
			log.Fatal(err)
			w.WriteHeader(500)
			w.Write([]byte(err.Error()))
		}
		metadatajson = append(metadatajson, jsonstring...)
		metadatajson = append(metadatajson, []byte(",")...)
	}

	// replace last commata with ] to create valid json Array
	metadatajson[len(metadatajson)-1] = byte(']')

	// Write Response with default 200 OK Status Code
	w.Write(metadatajson)
}

// nameFilter sets all Elements in datasets to nil when string does not match
func nameFilter(datasets []os.FileInfo, name string) {
	for index := range datasets {
		match, _ := regexp.MatchString(name, datasets[index].Name())
		if !match {
			datasets[index] = nil
		}
	}
}

// metaDataFilter sets all Elements in datasets to nil when generationTime is not withing bounds set by startDate and endDate or does not intersect bbox.
func metaDataFilter(datasets []os.FileInfo, startDateRAW, endDateRAW string, bbox *geos.Geometry, filterDates, filterBox bool) error {
	var startDate, endDate time.Time
	if filterDates {
		var err, err2 error
		startDate, err = time.Parse(time.RFC3339, startDateRAW)
		endDate, err2 = time.Parse(time.RFC3339, endDateRAW)
		if err != nil || err2 != nil {
			log.Fatal("Invalid startDate/endDate supplied ", err, err2)
			return err
		}
	}
	for index := range datasets {
		dataset, err := gdal.Open("/opt/sentinel2/"+datasets[index].Name()+"/MTD_MSIL1C.xml", gdal.ReadOnly)
		if err != nil {
			log.Fatal(err)
			return err
		}
		// Get Metadata
		generationTimeRAW := dataset.Driver().MetadataItem("GENERATION_TIME", "")
		footprintRAW := dataset.Driver().MetadataItem("FOOTPRINT", "")

		if filterDates {
			// Conversion to usable Time
			generationTime, err := time.Parse(time.RFC3339, generationTimeRAW)
			if err != nil {
				log.Fatal(err)
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
				log.Fatal(err)
				return err
			}

			// Check if Dataset overlaps with bbox
			intersects, err := footprint.Intersects(bbox)
			if err != nil {
				log.Fatal(err)
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
func getMetaData(datasets []os.FileInfo, page int, metadata chan []string) (metadatacount int, error error) {
	// Total counts of elements found in datasets
	totalcounter := 0
	// Number of Elements pushed into channel
	metadatacounter := 0
	pagesize := cap(metadata)

	for index := range datasets {
		// Fetch Metadata if Dataset is not sorted out + is on correct page
		if datasets[index] != nil {
			if metadatacounter == pagesize {
				// Pushed all to channel
				return metadatacounter, nil
			}
			totalcounter++
			// Push to metadata if on correct page and not already full
			if totalcounter > page*pagesize && metadatacounter < pagesize {
				metadatacounter++
				dataset, err := gdal.Open(
					"/opt/sentinel2/"+datasets[index].Name()+"/MTD_MSIL1C.xml",
					gdal.ReadOnly)
				if err != nil {
					log.Fatal(err)
					return metadatacounter, err
				}
				metadata <- append(dataset.Metadata(""), dataset.Metadata("Subdatasets")...)
				dataset.Close()
			}
		}
	}
	return metadatacounter, nil
}
