package main

import (
	"github.com/julienschmidt/httprouter"
	"github.com/ksshannon/go-gdal"
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
	q := r.URL.Query()
	datasets, err := ioutil.ReadDir("/opt/sentinel2")
	if err != nil {
		log.Fatal("unable to read data directory ", err)
	}

	bboxstring := q.Get("bbox")
	coordinates := strings.Split(bboxstring, ",")
	// Create geometry from bbox
	ll1, err := strconv.ParseFloat(coordinates[0], 64)
	ll2, err2 := strconv.ParseFloat(coordinates[1], 64)
	ur1, err3 := strconv.ParseFloat(coordinates[2], 64)
	ur2, err4 := strconv.ParseFloat(coordinates[3], 64)

	bbox, err5 := geos.NewPolygon([]geos.Coord{geos.NewCoord(ll1, ll2), geos.NewCoord(ur1, ur2)})
	if err != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil {
		log.Fatal("failed to parse bbox ", err, err2, err3, err4, err5)
		//TODO(specki) Return HTTP error Response
	}

	// Filter by Name
	nameFilter(datasets, q.Get("substring"))

	// Filter by bbox, startDate, endDate
	metaDataFilter(datasets, "2006-01-02T15:00:00Z", "2006-01-02T15:04:05Z", bbox)

	//TODO(specki) return first 10 non-nil elements from datasets with their metadata
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
// TODO(specki) search parameters are optional
func metaDataFilter(datasets []os.FileInfo, startDateRAW, endDateRAW string, bbox *geos.Geometry) error {

	startDate, err := time.Parse(time.RFC3339, startDateRAW)
	endDate, err2 := time.Parse(time.RFC3339, endDateRAW)
	if err != nil || err2 != nil {
		log.Fatal("Invalid startDate/endDate supplied ", err, err2)
		return err
	}

	for index := range datasets {
		dataset, err := gdal.Open(datasets[index].Name()+"/MTD_MSIL1C.xml", gdal.ReadOnly)
		if err != nil {
			log.Fatal(err)
			return err
		}
		// Get Metadata
		generationTimeRAW := dataset.MetadataItem("GENERATION_TIME", "")
		footprintRAW := dataset.MetadataItem("FOOTPRINT", "")

		// Conversion to usable Time
		generationTime, err := time.Parse(time.RFC3339, generationTimeRAW)
		if err != nil {
			log.Fatal(err)
			return err
		}
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
		if generationTime.Before(startDate) || generationTime.After(endDate) || !intersects {
			datasets[index] = nil
		}

		// Close Dataset
		dataset.Close()
	}
	return nil
}
