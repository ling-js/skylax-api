package main

import (
	"fmt"
	"github.com/ksshannon/go-gdal"
	"log"
	"time"
)

// Tracks the time elapsed since start.
func timeTrack(start time.Time, name string) {
	elapsed := time.Since(start)
	fmt.Printf("%s took %s\n", name, elapsed)
}

func main3() {
	defer timeTrack(time.Now(), "main")

	//DEBUG ONLY
	filename := "SENTINEL2_L1C:S2A_MSIL1C_20171019T111051_N0205_R137_T31UCT_20171019T111235.SAFE/MTD_MSIL1C.xml:10m:EPSG_32631"

	ch := make(chan []uint16, 3)

	go ReadDataFromDataset(1, filename, ch)
	a := <-ch
	go ReadDataFromDataset(2, filename, ch)
	b := <-ch
	go ReadDataFromDataset(3, filename, ch)
	c := <-ch

	defer timeTrack(time.Now(), "go routine started")

	writeGeoTIFF(filename, a, b, c, 0, 65536, 0, 65536, 0, 65536)

}

// TODO(specki) refactor to load multiple bands if they are in same subdataset instead of loading them seperately
// TODO(specki) Bandname (B02) to bandnumber (1-indexed)
func ReadDataFromDataset(bandnumber int, filename string, ch chan []uint16) error {
	defer timeTrack(time.Now(), "Reading Data from Dataset "+filename)
	// Open Dataset via GDAL
	dataset, err := gdal.Open(filename, gdal.ReadOnly)
	if err != nil {
		log.Fatal(err)
		return err
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
		log.Fatal(err)
		return err
	}

	// return filled buffer via channel
	ch <- b
	return nil
}

//TODO(specki) Allow 1 Band for Greyscale images
//TODO(specki) Images with different resolutions
func writeGeoTIFF(
	inputdataset string,
	red, green, blue []uint16,
	minred, maxred, mingreen, maxgreen, minblue, maxblue float64,
) error {
	defer timeTrack(time.Now(), "WriteGeoTIFF: ")

	// Open original file to copy Georeference
	original, err := gdal.Open(inputdataset, gdal.ReadOnly)
	if err != nil {
		log.Fatal(err)
		return err
	}
	defer original.Close()

	rastersize := original.RasterXSize()

	//fmt.Println(original.GeoTransform())
	//fmt.Print(original.ProjectionRef())

	driver, err := gdal.GetDriverByName("GTiff")
	if err != nil {
		log.Fatal(err)
		return err
	}
	newdataset := driver.Create(
		"test.tif",
		rastersize,
		rastersize,
		3,
		gdal.Byte,
		[]string{"INTERLEAVE=BAND"})
	defer newdataset.Close()

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
