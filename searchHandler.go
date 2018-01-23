package main

import (
	"encoding/json"
	"fmt"
	"github.com/ling-js/go-gdal"
	"github.com/paulsmith/gogeos/geos"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// custom Interface to sort by Date
type Sentinel2Dataset []os.FileInfo
func (nf Sentinel2Dataset) Len() int {return len(nf)}
func (nf Sentinel2Dataset) Swap(i,j int) {nf[i], nf[j] = nf[j], nf[i] }
func (nf Sentinel2Dataset) Less(i, j int) bool {
	// Compare names from 12th letter onwards lexicographically
	return nf[i].Name()[11:] < nf[j].Name()[11:]
}

// Searchhandler returns all Datasets not matching one of the filter criteria.
func SearchHandler(w http.ResponseWriter, r *http.Request) {
	defer Timetrack(time.Now(), "Search ")
	q := r.URL.Query()
	// Get all Datasets from Directory
	datasets, err := ioutil.ReadDir("/opt/sentinel2")
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte("Unable to open Data Repository: " + err.Error()))
		return
	}
	// Sort by Date
	sort.Sort(Sentinel2Dataset(datasets))

	bboxstring := q.Get("bbox")
	var bbox *geos.Geometry

	// Parse supplied coordinates into bbox
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

	// Get Filter Filter by Name
	err = nameFilter(datasets, q.Get("substring"))
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte("Unable to filter by substring: " + err.Error()))
		return
	}

	// Setup filter by bbox, startDate, endDate
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

	// Prepare metadata output
	var metadatajson []byte

	// Get metadata
	mtdL1C, mtdL2A, totalcounter, err := getMetaData(datasets, page)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte("Unable to retrieve Metadata " + err.Error()))
		return
	}

	// Write max page into response Headers
	w.Header().Set("X-Dataset-Count", strconv.Itoa(totalcounter))
	w.Header().Set("Access-Control-Expose-Headers", "X-Dataset-Count")

	// Set content-type header
	w.Header().Set("Content-Type", "application/json")

	// Create metadata
	metadatajson = append(metadatajson, mtdL1C[:len(mtdL1C)-1]...)
	metadatajson = append(metadatajson, []byte("],")...)
	metadatajson = append(metadatajson, mtdL2A[:len(mtdL2A)-1]...)
	metadatajson = append(metadatajson, []byte("]}")...)

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
			return err
		}
		if err2 != nil {
			return err2
		}
	}
	for index := range datasets {
		// Only deal with existent datasets
		if datasets[index] == nil {
			return nil
		}

		var generationTimeRAW, footprintRAW string
		var dataset *gdal.Dataset
		var err error

		// Try to get L1C Metadata
		dataset, err = gdal.Open("/opt/sentinel2/"+datasets[index].Name()+"/MTD_MSIL1C.xml", gdal.ReadOnly)
		if err == nil {
			// Get Metadata via hardcoded position
			generationTimeRAW = dataset.Metadata("")[12][16:]
			footprintRAW = dataset.Metadata("")[9][10:]
		} else {
			// Else Try to get S2A Metadata
			dataset, err = gdal.Open("/opt/sentinel2/"+datasets[index].Name()+"/MTD_MSIL2A.xml", gdal.ReadOnly)
			if err == nil {
				// Get Metadata via hardcoded position
				generationTimeRAW = dataset.Metadata("")[17][16:]
				footprintRAW = dataset.Metadata("")[14][10:]
			} else {
				return err
			}
		}

		// Apply Date Filter if selected
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

// Gets the metaData for 8 items starting with element page*8.
func getMetaData(datasets []os.FileInfo, page int) (metadataL1C, metadataL2A []byte, totalcounter int, error error) {
	// Start assembling Metadata
	metadataL1C = []byte("{\"L1C\":[ ")
	metadataL2A = []byte("\"L2A\":[ ")
	// If L2A Dataset add additional Metadata
	var L2A bool
	// Total counts of elements found in datasets
	totalcounter = 0
	// Number of Elements pushed into channel
	metadatacounter := 0
	// Number of Elements per Page
	pagesize := 8

	for index := range datasets {
		// Fetch Metadata if Dataset is not sorted out + is on correct page
		if datasets[index] != nil {
			totalcounter++
			// Push to metadata if on correct page and not already full
			if totalcounter > page*pagesize && metadatacounter < pagesize {
				metadatacounter++

				// Try to open and read Metadata of L1C Dataset
				dataset, err := gdal.Open(
					"/opt/sentinel2/"+datasets[index].Name()+"/MTD_MSIL1C.xml",
					gdal.ReadOnly)
				if err == nil {
					L2A = false
					createJSON(append(dataset.Metadata(""), dataset.Metadata("Subdatasets")...), &metadataL1C)
					dataset.Close()
				} else {
					// Try to open and read Metadata of L2A Dataset
					dataset, err := gdal.Open(
						"/opt/sentinel2/"+datasets[index].Name()+"/MTD_MSIL2A.xml",
						gdal.ReadOnly)
					if err == nil {
						L2A = true
						createJSON(dataset.Metadata(""), &metadataL2A)
						dataset.Close()
					} else {
						return nil, nil, 0, err
					}
				}
				if L2A {
					// Get datast location (with dynamic folder name)
					datalocation := "/opt/sentinel2/" + datasets[index].Name() + "/GRANULE/"
					datasetname, err := ioutil.ReadDir(datalocation)
					if err != nil {
						return nil, nil, 0, err
					}
					//
					location := datalocation + datasetname[0].Name()
					datasetsR10M, err := ioutil.ReadDir(location + "/IMG_DATA/R10m/")
					datasetsR20M, err := ioutil.ReadDir(location + "/IMG_DATA/R20m/")
					datasetsR60M, err := ioutil.ReadDir(location + "/IMG_DATA/R60m/")

					var datasetsR10Mstring string
					for i := range datasetsR10M {
						datasetsR10Mstring += datasetsR10M[i].Name() + "\",\""
					}
					var datasetsR20Mstring string
					for i := range datasetsR20M {
						datasetsR20Mstring += datasetsR20M[i].Name() + "\",\""
					}
					var datasetsR60Mstring string
					for i := range datasetsR60M {
						datasetsR60Mstring += datasetsR60M[i].Name() + "\",\""
					}

					metadataL2A = metadataL2A[:len(metadataL2A)-2]
					metadataL2A = append(metadataL2A, []byte(",\"R10M\":[\""+datasetsR10Mstring[:len(datasetsR10Mstring)-2]+"]")...)
					metadataL2A = append(metadataL2A, []byte(",\"R20M\":[\""+datasetsR20Mstring[:len(datasetsR20Mstring)-2]+"]")...)
					metadataL2A = append(metadataL2A, []byte(",\"R60M\":[\""+datasetsR60Mstring[:len(datasetsR60Mstring)-2]+"]},")...)
				}
			}
		}
	}
	return metadataL1C, metadataL2A, totalcounter, nil
}
// Creates a JSON Object as byte slice from gdalinfo output
func createJSON(input []string, output *[]byte) error {
	// Convert into JSON
	fields := make(map[string]string)
	for index := range input {
		keyValuePair := strings.Split(input[index], "=")
		fields[keyValuePair[0]] = keyValuePair[1]
	}
	jsonstring, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	jsonstring = append(jsonstring, []byte(",")...)
	*output = append(*output, jsonstring...)
	return nil
}
