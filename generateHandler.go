package main

import (
	"fmt"
	"github.com/gorilla/schema"
	"github.com/ling-js/go-gdal"
	"github.com/pkg/errors"
	"github.com/segmentio/ksuid"
	"math"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Tracks the time elapsed since start.
func Timetrack(start time.Time, name string) {
	elapsed := time.Since(start)
	fmt.Printf("%s finished in %s\n", name, elapsed)
}

type options struct {
	Rgbbool bool    `schema:"rgbbool"`
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

func parseOptions(r *http.Request) (options options, err error) {
	err = r.ParseForm()

	if err != nil {
		return options, err
	}
	decoder := schema.NewDecoder()
	err = decoder.Decode(&options, r.PostForm)
	if err != nil {
		return options, err
	}
	var ksu, err2 = ksuid.NewRandom()
	if err2 != nil {
		return options, err
	}

	options.id = ksu.String()
	return options, nil
}

func GenerateHandler(w http.ResponseWriter, r *http.Request) {
	defer Timetrack(time.Now(), "GenerateHandler ")

	options, err := parseOptions(r)

	if err != nil {
		w.WriteHeader(400)
		w.Write([]byte("Unable to parse parameters: " + err.Error()))

		if Verbose {
			fmt.Println("Error getting the Options from the Request")
			fmt.Println(err.Error())
		}
		return
	}

	if options.Rgbbool {
		//TODO(specki): refactor into goroutines or refactor to pointers instead of channels
		ch := make(chan []uint16, 3)
		// Read source data
		err = ReadDataFromDataset(options.Rcn, options.Rcdn, ch, w)
		r := <-ch
		if err != nil {
			if Verbose {
				fmt.Println("Error reading red dataset")
				fmt.Println(err.Error())
			}
			return
		}
		err = ReadDataFromDataset(options.Gcn, options.Gcdn, ch, w)
		g := <-ch
		if err != nil {
			if Verbose {
				fmt.Println("Error reading green dataset")
				fmt.Println(err.Error())
			}
			return
		}
		err = ReadDataFromDataset(options.Bcn, options.Bcdn, ch, w)
		b := <-ch
		if err != nil {
			if Verbose {
				fmt.Println("Error reading blue dataset")
				fmt.Println(err.Error())
			}
			return
		}

		err = writeGeoTIFF_RGB(
			options.Gcdn, // copy georeference
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
			return
		}
	} else {
		ch := make(chan []uint16, 1)
		// Read source data
		err = ReadDataFromDataset(options.Gsc, options.Gscdn, ch, w)
		g := <-ch
		if err != nil {
			if Verbose {
				fmt.Println("Error reading grey dataset")
				fmt.Println(err.Error())
			}
			return
		}
		err = writeGeoTIFF_GREY(
			options.Gscdn,
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
			return
		}
	}

	// calculate correct NODATA values
	var nodata string
	if options.Rgbbool {
		nodata = "0,0,0"
	} else {
		nodata = "0"
	}

	// Tiling via gdal2tiles
	cmd := exec.Command("./DEV_gdal2tiles.py", "--resume", "-z", "9-12", "-w", "none", "-a", nodata, options.id+".tif", "data/"+options.id+"/")
	fmt.Println(cmd)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Run()

	// 200 Response with generated ID
	w.Write([]byte(options.id))
}

func ReadDataFromDataset(bandname, filename string, ch chan []uint16, w http.ResponseWriter) error {
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
		ch <- make([]uint16, 0)
		return err
	}

	// map bandname to appropiate bandnumber
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
			ch <- make([]uint16, 0)
			return err
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
		ch <- make([]uint16, 0)
		return errors.New("dummy")
	}

	// defer closing until function exit
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
		ch <- make([]uint16, 0)
		return err
	}

	// return filled buffer via channel
	ch <- b
	return nil
}

func writeGeoTIFF_GREY(
	inputdataset, outputdataset string,
	grey []uint16,
	mingrey, maxgrey float64,
) error {
	newdataset, rastersize, err := createGeoTIFF(inputdataset, outputdataset, 1)
	if err != nil {
		return err
	}

	var data8bit = make([]byte, len(grey))
	transformColorValues(data8bit, grey, maxgrey, mingrey, len(grey))

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
	newdataset.Close()
	return nil
}

// creates new GeoTIFF with same georeference as inputdataset.
func createGeoTIFF(inputdataset, outputdataset string, bandcount int) (*gdal.Dataset, int, error) {
	// Open original file to copy Georeference
	original, err := gdal.Open(inputdataset, gdal.ReadOnly)
	if err != nil {
		return nil, 0, err
	}
	defer original.Close()

	rastersize := original.RasterXSize()

	driver, err := gdal.GetDriverByName("GTiff")
	if err != nil {
		return nil, 0, err
	}

	newdataset := driver.Create(
		outputdataset,
		rastersize,
		rastersize,
		bandcount,
		gdal.Byte,
		[]string{"INTERLEAVE=BAND"})

	newdataset.SetGeoTransform(original.GeoTransform())
	newdataset.SetProjection(original.ProjectionRef())

	return newdataset, rastersize, nil
}

// transformColorValues transforms given 16-bit values into 8-bit values.
// Linear transform unless original values are outside given bounds, then 0
func transformColorValues(output []uint8, data []uint16, maxvalue, minvalue float64, newsize int) {
	delta := sliceDelta(data)
	originalrowsize := int(math.Sqrt(float64(len(data))))
	newrowsize := int(math.Sqrt(float64(newsize)))

	var runnerdelta int
	switch factor := (newrowsize - originalrowsize) / 1830; factor {
	case 0:
		runnerdelta = -999
	case 2:
		runnerdelta = 3
	case 3:
		runnerdelta = 2
	case 5:
		runnerdelta = 6
	}

	// Optimize if newscale is same as oldscale
	if runnerdelta != -999 {
		runner := 0
		runnerY := runnerdelta
		runnerX := runnerdelta

		for i := 0; i < newsize; i++ {
			// Check if we are at end of line in matrix
			if i%(newrowsize-1) == 0 && i != 0 {
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

func writeGeoTIFF_RGB(
	inputdataset, outputdataset string,
	red, green, blue []uint16,
	minred, maxred, mingreen, maxgreen, minblue, maxblue float64,
) error {
	defer Timetrack(time.Now(), "WriteGeoTIFF: ")

	newdataset, rastersize, err := createGeoTIFF(inputdataset, outputdataset, 3)
	if err != nil {
		return err
	}

	// 8bit data
	var data8bit = make([]byte, 361681200)

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

	// for red channel
	transformColorValues(data8bit[:120560400], red, maxred, minred, maxresolution)
	transformColorValues(data8bit[120560400:241120800], green, maxgreen, mingreen, maxresolution)
	transformColorValues(data8bit[241120800:], blue, maxblue, minblue, maxresolution)

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

// sliceDelta return the difference between largest and smallest number in slice
//TODO(specki): Maybe refactor to use ComputeMinMax() of GDAL Rasterband
func sliceDelta(slice []uint16) (delta float64) {
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
