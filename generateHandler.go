package main

import (
	"errors"
	"fmt"
	"github.com/gorilla/schema"
	"github.com/ling-js/go-gdal"
	"github.com/segmentio/ksuid"
	"io/ioutil"
	"math"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// HTTP-POST Body options
type options struct {
	Rgbbool bool    `schema:"rgbbool"`
	S2A     bool    `schema:"l2a"`
	TCI     bool    `schema:"tci"`
	id      string  `schema:"-"`
	Gscdn   string  `schema:"gscdn"`
	Rcdn    string  `schema:"rcdn"`
	Gcdn    string  `schema:"gcdn"`
	Bcdn    string  `schema:"bcdn"`
	Gsc     string  `schema:"gsc"`
	Rcn     string  `schema:"rcn"`
	Gcn     string  `schema:"gcn"`
	Bcn     string  `schema:"bcn"`
	Greymin float64 `schema:"greymin"`
	Rcmin   float64 `schema:"rcmin"`
	Gcmin   float64 `schema:"gcmin"`
	Bcmin   float64 `schema:"bcmin"`
	Greymax float64 `schema:"greymax"`
	Rcmax   float64 `schema:"rcmax"`
	Gcmax   float64 `schema:"gcmax"`
	Bcmax   float64 `schema:"bcmax"`
}

// GenerateHandler handles all Requests for Dataset Generation
func GenerateHandler(w http.ResponseWriter, r *http.Request) {
	defer Timetrack(time.Now(), "GenerateHandler ")

	options, err := parseOptions(r)

	// Log request if verbose is set
	if Verbose {
		fmt.Print("Request to /generate with following parameters: ")
		fmt.Println(options)
	}

	if err != nil {
		w.WriteHeader(400)
		w.Write([]byte("Unable to parse parameters: " + err.Error()))

		if Verbose {
			fmt.Println("Error getting the Options from the Request")
			fmt.Println(err.Error())
		}
		return
	}
	// Get Name of original Dataset for later georeferencing
	var originalDataset string

	// Get Name of original Dataset
	if options.S2A {
		var resolution string
		var datasetname string

		// Get Resolution + Set base Dataset
		if options.Rgbbool {
			resolution = options.Rcn[len(options.Rcn)-7 : len(options.Rcn)-5]
			datasetname = options.Rcdn
		} else {
			resolution = options.Gsc[len(options.Gsc)-7 : len(options.Gsc)-5]
			datasetname = options.Gscdn
		}

		// Get Name of dynamically named subfolder
		datalocation := DataSource + datasetname + "/GRANULE/"
		subfolder, err := ioutil.ReadDir(datalocation)
		if err != nil {
			w.WriteHeader(400)
			w.Write([]byte("Unable to find dataset: " + err.Error()))

			if Verbose {
				fmt.Println("Error finding the Dataset")
				fmt.Println(err.Error())
			}
		}

		// get name of Dataset to copy georeference
		originalDataset = DataSource + datasetname + "/GRANULE/" + subfolder[0].Name() + "/IMG_DATA/R" + resolution + "m/"
		if options.Rgbbool {
			originalDataset += options.Rcn
		} else {
			originalDataset += options.Gsc
		}
	} else {
		if options.Rgbbool {
			originalDataset = options.Rcdn
		} else {
			originalDataset = options.Gscdn
		}
	}

	// Read Data from source datasets
	if options.Rgbbool {
		err = HandleRGB(originalDataset, options, w)
	} else if options.TCI {
		err = HandleTCI(originalDataset, options, w)
	} else {
		err = HandleGSC(originalDataset, options, w)
	}
	if err != nil {
		return
	}

	// choose correct NODATA values
	var nodata string
	if options.Rgbbool {
		nodata = "0,0,0"
	} else {
		nodata = "0"
	}

	// TCI Tiling is done separately
	if !options.TCI {
		// Tiling via gdal2tiles
		cmd := exec.Command("./gdal2tiles.py", "--resume", "-z", "4-12", "-w", "none", "-a", nodata, options.id+".tif", "data/"+options.id+"/")
		cmd.Run()
	}
	// 200 Response with generated ID
	w.Write([]byte(options.id))
}

// HandleRGB handles creation of RGB Images from user-supplied Input Datasets
func HandleRGB(originalDataset string, options options, w http.ResponseWriter) error {
	var r, g, b []uint16
	var err error

	// Read red dataset
	if !options.S2A {
		r, err = ReadDataFromDatasetL1C(options.Rcn, options.Rcdn, w)
	} else {
		r, err = ReadDataFromDatasetL2A(options.Rcn, options.Rcdn, w)
	}
	if err != nil {
		if Verbose {
			fmt.Println("Error reading red dataset")
			fmt.Println(err.Error())
		}
		return err
	}

	// Read green dataset
	if !options.S2A {
		g, err = ReadDataFromDatasetL1C(options.Gcn, options.Gcdn, w)
	} else {
		g, err = ReadDataFromDatasetL2A(options.Gcn, options.Gcdn, w)
	}
	if err != nil {
		if Verbose {
			fmt.Println("Error reading green dataset")
			fmt.Println(err.Error())
		}
		return err
	}

	// Read blue dataset
	if !options.S2A {
		b, err = ReadDataFromDatasetL1C(options.Bcn, options.Bcdn, w)
	} else {
		b, err = ReadDataFromDatasetL2A(options.Bcn, options.Bcdn, w)
	}
	if err != nil {
		if Verbose {
			fmt.Println("Error reading blue dataset")
			fmt.Println(err.Error())
		}
		return err
	}

	// Write all data to temporary tif file
	err = writeGeoTiffRGB(
		originalDataset, // copy georeference
		options.id+".tif",
		r,
		g,
		b,
		options.Rcmin,
		options.Rcmax,
		options.Gcmin,
		options.Gcmax,
		options.Bcmin,
		options.Bcmax)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte("Unable to generate RGB image: " + err.Error()))
		if Verbose {
			fmt.Println("Error writing data to temporary GeoTIFF File")
			fmt.Println(err.Error())
		}
		return err
	}
	return nil
}

// HandleGSC handles creation of Greyscale Images from user-supplied Input Dataset
func HandleGSC(originalDataset string, options options, w http.ResponseWriter) error {
	// Read source data
	var g []uint16
	var err error

	if !options.S2A {
		g, err = ReadDataFromDatasetL1C(options.Gsc, options.Gscdn, w)
	} else {
		g, err = ReadDataFromDatasetL2A(options.Gsc, options.Gscdn, w)
	}
	if err != nil {
		if Verbose {
			fmt.Println("Error reading grey dataset")
			fmt.Println(err.Error())
		}
		return err
	}

	// Write Data to .tif
	err = writeGeoTiffGrey(
		originalDataset,
		options.id+".tif",
		g,
		options.Greymin,
		options.Greymax)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte("Unable to generate Greyscale image: " + err.Error()))
		if Verbose {
			fmt.Println("Error writing data to temporary GeoTIFF File")
			fmt.Println(err.Error())
		}
		return err
	}
	return nil
}

// HandleTCI handles Request for True Color Images
func HandleTCI(originalDataset string, options options, w http.ResponseWriter) error {

	// Get jp2 location from L1C Dataset
	if !options.S2A {
		dataset, err := gdal.Open(originalDataset, gdal.ReadOnly)
		if err != nil {
			w.WriteHeader(500)
			w.Write([]byte("Error opening Dataset: " + err.Error()))
			if Verbose {
				fmt.Println("Error opening Dataset by GDAL")
				fmt.Println(err.Error())
			}
			return err
		}
		// Get jp2 Location
		originalDataset = dataset.FileList()[2]
	}

	if Verbose {
		fmt.Println("Running Tiling Script for Dataset " + originalDataset + "...")
	}
	// Tiling via gdal2tiles
	cmd := exec.Command("./gdal2tiles.py", "--resume", "-v", "-z", "4-12", "-w", "none", "-a", "0,0,0", originalDataset, "data/"+options.id+"/")
	cmd.Run()
	if Verbose {
		fmt.Println("... Tiling Script finished.")
	}
	return nil
}

// ReadDataFromDatasetL2A reads a Sentinel Level 2A Dataset into uint16 slice
func ReadDataFromDatasetL2A(datasetname, filename string, w http.ResponseWriter) ([]uint16, error) {
	defer Timetrack(time.Now(), "Reading Data from Dataset "+filename)

	// Get Name of dynamically named subfolder
	datalocation := DataSource + filename + "/GRANULE/"
	subfolder, err := ioutil.ReadDir(datalocation)
	if err != nil {
		return nil, err
	}

	// Get Resolution
	resolution := datasetname[len(datasetname)-7 : len(datasetname)-5]

	//Open Dataset via GDAL
	dataset, err := gdal.Open(DataSource+filename+"/GRANULE/"+subfolder[0].Name()+"/IMG_DATA/R"+resolution+"m/"+datasetname, gdal.ReadOnly)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte("Error opening Dataset: " + err.Error()))
		if Verbose {
			fmt.Println("Error opening Dataset by GDAL")
			fmt.Println(err.Error())
		}
	}
	defer dataset.Close()

	// get dimensions
	rasterSize := dataset.RasterXSize()
	// create Data buffer
	b := make([]uint16, rasterSize*rasterSize)

	// Read data from dataset
	err = dataset.IO(
		gdal.Read,
		0,
		0,
		rasterSize,
		rasterSize,
		b,
		rasterSize,
		rasterSize,
		1,
		[]int{1},
		0,
		0,
		0)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte("Error reading data from Dataset: " + err.Error()))
		if Verbose {
			fmt.Println("Error reading data from dataset")
			fmt.Println(err.Error())
		}
		return nil, err
	}
	// return filled data slice
	return b, nil
}

// ReadDataFromDatasetL1C reads a Sentinel Level 1C Dataset into uint16 slice
func ReadDataFromDatasetL1C(bandname, filename string, w http.ResponseWriter) ([]uint16, error) {
	defer Timetrack(time.Now(), "Reading Data from Dataset "+filename)
	// Open Dataset via GDAL
	dataset, err := gdal.Open(filename, gdal.ReadOnly)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte("Error opening Dataset: " + err.Error()))
		if Verbose {
			fmt.Println("Error opening Dataset by GDAL")
			fmt.Println(err.Error())
		}
		return nil, err
	}

	// map bandname to appropriate bandnumber
	rasterbands := dataset.RasterCount()
	var bandnumber int
	for i := 1; i <= rasterbands; i++ {
		layer, err := dataset.RasterBand(i)
		if err != nil {
			w.WriteHeader(500)
			w.Write([]byte("Error reading rasterband from dataset: " + err.Error()))
			if Verbose {
				fmt.Println("Error reading rasterband from dataset")
				fmt.Println(err.Error())
			}
			return nil, err
		}
		bandstring := strings.Split(layer.Metadata("")[0], "=")
		if bandstring[1] == bandname {
			bandnumber = i
		}
	}
	// check if bandnumber is valid - else invalid bandname was supplied
	if bandnumber == 0 {
		w.WriteHeader(400)
		w.Write([]byte("Invalid Bandname supplied. Band '" + bandname + "' does not exist in Dataset " + filename))
		if Verbose {
			fmt.Println("Invalid Bandname supplied. Band '" + bandname + "' does not exist in Dataset " + filename)
		}
		return nil, errors.New("dummy")
	}
	defer dataset.Close()

	// get dimensions
	rasterSize := dataset.RasterXSize()
	// create Data buffer
	b := make([]uint16, rasterSize*rasterSize)

	// Read Data
	err = dataset.IO(
		gdal.Read,
		0,
		0,
		rasterSize,
		rasterSize,
		b,
		rasterSize,
		rasterSize,
		1,
		[]int{bandnumber},
		0,
		0,
		0)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte("Error reading data from Dataset: " + err.Error()))
		if Verbose {
			fmt.Println("Error reading data from dataset")
			fmt.Println(err.Error())
		}
		return nil, err
	}
	// return filled buffer
	return b, err
}

// writeGeoTiffGrey creates a new TIF File with given data mapped to given bounds
func writeGeoTiffGrey(
	inputdataset, outputdataset string,
	grey []uint16,
	mingrey, maxgrey float64,
) error {
	newdataset, rastersize, err := createGeoTIFF(inputdataset, outputdataset, 1)
	if err != nil {
		return err
	}
	defer newdataset.Close()

	// Map original Values to 0-255 space
	var data8bit = make([]byte, len(grey))
	transformColorValues(data8bit, grey, maxgrey, mingrey, len(grey))

	// Write to File
	newdataset.IO(
		gdal.Write,
		0,
		0,
		rastersize,
		rastersize,
		data8bit,
		rastersize,
		rastersize,
		1,
		[]int{1},
		0,
		0,
		0,
	)
	return nil
}

// writeGeoTiffRGB creates a new GeoTIFF File and writes provided r g b values to it
func writeGeoTiffRGB(
	inputdataset, outputdataset string,
	red, green, blue []uint16,
	minred, maxred, mingreen, maxgreen, minblue, maxblue float64,
) error {
	defer Timetrack(time.Now(), "WriteGeoTIFF: ")

	// Create base File
	newdataset, rastersize, err := createGeoTIFF(inputdataset, outputdataset, 3)
	if err != nil {
		return err
	}

	// get size of dataset with highest resolution
	maxresolution := 0
	lenblue := len(blue)
	lenred := len(red)
	lengreen := len(green)

	if lenred > lengreen && lenred > lenblue {
		maxresolution = lenred
	} else if lengreen > lenred && lengreen > lenblue {
		maxresolution = lengreen
	} else {
		maxresolution = lenblue
	}

	// temporary container for output data
	var data8bit = make([]byte, maxresolution*3)

	// Transform all Color values to 0-255 space
	transformColorValues(data8bit[:maxresolution], red, maxred, minred, maxresolution)
	transformColorValues(data8bit[maxresolution:2*maxresolution], green, maxgreen, mingreen, maxresolution)
	transformColorValues(data8bit[2*maxresolution:], blue, maxblue, minblue, maxresolution)

	// Write Data to file
	newdataset.IO(
		gdal.Write,
		0,
		0,
		rastersize,
		rastersize,
		data8bit,
		rastersize,
		rastersize,
		3,
		[]int{1, 2, 3},
		0,
		0,
		0,
	)
	newdataset.Close()
	return nil
}

// creates new GeoTIFF with same georeference as inputdataset.
func createGeoTIFF(inputdataset, outputdataset string, bandcount int) (*gdal.Dataset, int, error) {

	// Open original file to get Georeference
	original, err := gdal.Open(inputdataset, gdal.ReadOnly)
	if err != nil {
		return nil, 0, err
	}
	defer original.Close()

	// Copy original size to new Dataset
	rastersize := original.RasterXSize()

	driver, err := gdal.GetDriverByName("GTiff")
	if err != nil {
		return nil, 0, err
	}

	// Create new file and write Dataset
	newdataset := driver.Create(
		outputdataset,
		rastersize,
		rastersize,
		bandcount,
		gdal.Byte,
		[]string{"INTERLEAVE=BAND"})

	// Copy Georeference to new dataset
	newdataset.SetGeoTransform(original.GeoTransform())
	newdataset.SetProjection(original.ProjectionRef())

	return newdataset, rastersize, nil
}

// transformColorValues transforms given 16-bit values into 8-bit values.
// If input size is smaller than the desired output size the input is scaled to output by value duplication
// Linear transform unless original values are outside given bounds, then 0
func transformColorValues(output []uint8, data []uint16, maxvalue, minvalue float64, newsize int) {
	delta := sliceDelta(data)
	originalrowsize := int(math.Sqrt(float64(len(data))))
	newrowsize := int(math.Sqrt(float64(newsize)))

	// Get scaling Factor
	var runnerdelta int
	switch factor := (newrowsize - originalrowsize) / 1830; factor {
	case 0:
		// Shortcut to skip scaling
		runnerdelta = -999
	case 2:
		runnerdelta = 3
	case 3:
		runnerdelta = 2
	case 5:
		runnerdelta = 6
	}

	// Skip scaling if newscale is same as oldscale
	if runnerdelta != -999 {
		runner := 0
		runnerY := runnerdelta
		runnerX := runnerdelta

		for i := 0; i < newsize; i++ {
			// Check if we are at end of line in matrix
			if i%(newrowsize) == 0 && i != 0 {
				runnerY--
				// If runnerY is zero reset runner to start of line
				if runnerY != 0 {
					runner = runner - originalrowsize
					// Force X runner reset
					runnerX = 0
				} else {
					runnerY = runnerdelta
				}
			}

			// Reset X runner once 0 is reached (counting down from delta)
			if runnerX == 0 {
				runner++
				runnerX = runnerdelta
			}
			runnerX--

			// get original data
			c := float64(data[runner])
			// check if value is within bounds
			if c < minvalue || maxvalue < c {
				c = 0
			}
			// transform to 0-255 space
			output[i] = (byte)((c / delta) * 255)
		}
	} else {
		// Fast transform
		for i := 0; i < newsize; i++ {
			c := float64(data[i])
			// check if value is within bounds
			if c < minvalue || maxvalue < c {
				c = 0
			}
			// transform to 0-255 space
			output[i] = (byte)((c / delta) * 255)
		}
	}
}

// parseOptions parses HTTP-Post Body to options struct
func parseOptions(r *http.Request) (options options, err error) {
	// Parse POST-Body
	err = r.ParseForm()
	if err != nil {
		return options, err
	}

	// Parse POST-Body to options struct
	decoder := schema.NewDecoder()
	err = decoder.Decode(&options, r.PostForm)
	if err != nil {
		return options, err
	}

	// Create unique random id for temporary TMS
	var ksu, err2 = ksuid.NewRandom()
	if err2 != nil {
		return options, err
	}
	options.id = ksu.String()

	return options, nil
}

// sliceDelta return the difference between largest and smallest number in slice
func sliceDelta(slice []uint16) (delta float64) {
	defer Timetrack(time.Now(), "MinMaxComputation")
	var min, max uint16
	for _, element := range slice {
		if element < min {
			min = element
		} else if element > max {
			max = element
		}
	}
	return float64(max - min)
}
