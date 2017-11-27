package main

import (
	"fmt"
	"github.com/gorilla/schema"
	"github.com/julienschmidt/httprouter"
	"github.com/ling-js/go-gdal"
	"github.com/pkg/errors"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Tracks the time elapsed since start.
func Timetrack(start time.Time, name string) {
	elapsed := time.Since(start)
	fmt.Printf("%s took %s\n", name, elapsed)
}

type options struct {
	rgbbool bool
	id      string
	gscdn   string
	rcdn    string
	gcdn    string
	bcdn    string
	gsc     string
	rcn     string
	gcn     string
	bcn     string
	greymin float64
	rcmin   float64
	gcmin   float64
	bcmin   float64
	greymax float64
	rcmax   float64
	gcmax   float64
	bcmax   float64
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
	options.id = Ksuid.Next().String()
	return options, nil
}

func GenerateHandler(w http.ResponseWriter, r *http.Request) {
	defer Timetrack(time.Now(), "GenerateHandler ")

	options, err := parseOptions(r)
	if err != nil {
		w.WriteHeader(400)
		w.Write([]byte("Unable to parse parameters: " + err.Error()))
		return
	}

	//DEBUG ONLY
	options.rgbbool = true
	options.rcdn = "SENTINEL2_L1C:S2A_MSIL1C_20171019T111051_N0205_R137_T31UCT_20171019T111235.SAFE/MTD_MSIL1C.xml:10m:EPSG_32631"
	options.gcdn = "SENTINEL2_L1C:S2A_MSIL1C_20171019T111051_N0205_R137_T31UCT_20171019T111235.SAFE/MTD_MSIL1C.xml:10m:EPSG_32631"
	options.bcdn = "SENTINEL2_L1C:S2A_MSIL1C_20171019T111051_N0205_R137_T31UCT_20171019T111235.SAFE/MTD_MSIL1C.xml:10m:EPSG_32631"
	options.rcn = "B8"
	options.gcn = "B8"
	options.bcn = "B8"
	options.rcmin = 0
	options.gcmin = 0
	options.bcmin = 0
	options.rcmax = 13000
	options.gcmax = 13000
	options.bcmax = 13000

	if options.rgbbool {
		ch := make(chan []uint16, 3)
		// Read source data
		err = ReadDataFromDataset(options.rcn, options.rcdn, ch, w)
		r := <-ch
		if err != nil {
			return
		}
		err = ReadDataFromDataset(options.gcn, options.gcdn, ch, w)
		g := <-ch
		if err != nil {
			return
		}
		err = ReadDataFromDataset(options.bcn, options.bcdn, ch, w)
		b := <-ch
		if err != nil {
			return
		}

		//TODO(specki) different scales!
		err = writeGeoTIFF_RGB(
			options.gcdn, // copy georeference
			options.id+".tif",
			r,
			g,
			b,
			options.rcmin,
			options.rcmax,
			options.gcmin,
			options.gcmax,
			options.bcmin,
			options.bcmax)
		if err != nil {
			w.WriteHeader(500)
			w.Write([]byte("Unable to generate RGB image: " + err.Error()))
			return
		}
	} else {
		//TODO same thing for greyscale images
	}

	// Tiling via gdal2tiles
	cmd := exec.Command("./gdal2tiles_parallel.py", "-e", "-p", "raster", "--format=PNG", "--processes=16", options.id+".tif", "data/"+options.id+"/")
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
			w.Write([]byte("Error reading rasterband from Dataset: " + err.Error()))
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
	newdataset, rastersize, err := createGeoTIFF(inputdataset, outputdataset, 3)
	if err != nil {
		return err
	}
	// Calculate current value ranges
	deltagrey := sliceDelta(grey)

	// 8bit data
	var data8bit = make([]byte, len(grey))

	// Convert uint16 Data to 8bit
	for i := 0; i < len(grey); i++ {
		g := float64(grey[i])
		if g < mingrey || maxgrey < g {
			g = 0
		}
		data8bit[i] = (byte)((g / deltagrey) * 255)
	}

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

//TODO(specki) Images with different resolutions
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
	// Calculate current value ranges
	deltared := sliceDelta(red)
	deltagreen := sliceDelta(green)
	deltablue := sliceDelta(blue)

	// 8bit data
	var data8bit = make([]byte, len(red)*3)
	offset := len(red)

	// Convert uint16 Data to 8bit
	for i := 0; i < len(red); i++ {
		r := float64(red[i])
		g := float64(green[i])
		b := float64(blue[i])
		if r < minred || maxred < r {
			r = 0
		}
		if g < mingreen || maxgreen < g {
			g = 0
		}
		if b < minblue || maxblue < b {
			b = 0
		}
		data8bit[i] = (byte)((r / deltared) * 255)
		data8bit[i+offset] = (byte)((g / deltagreen) * 255)
		data8bit[i+(2*offset)] = (byte)((b / deltablue) * 255)
	}

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
