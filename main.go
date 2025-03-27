package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/tkrajina/gpxgo/gpx"
)

func usage() {
	fmt.Println("Merge GPX files into one, starting off from a master GPX file. But add points only when they are not close to others. Augment point with rough elevation data in case missing. Config data is read from a file, too.")
	fmt.Println("Usage: go run . gpx-merger.yaml output.gpx")
	fmt.Println("Usage: ./gpx-merger gpx-merger.yaml output.gpx")
	os.Exit(0)
}

func check(e error) {
	if e != nil {
		fmt.Println(e.Error())
		panic(e)
	}
}

func main() {
	// config
	if len(os.Args) < 3 {
		usage()
	}
	err = loadConfig(os.Args[1])
	check(err)

	// files
	outputFile := os.Args[2]
	gpxMaster, err := ReadGpxFile("", config.MasterFile)
	check(err)
	fmt.Printf("GPX Master: %d waypoints.\n", len(gpxMaster.Waypoints))
	fmt.Printf("Spatial grid width : %f km, using Euclidean distance calculation!\n", config.GridSizeInDegree*metersPerDegree/1000)
	check(err)
	fmt.Printf("Minimum distance between points : %f m.\n", config.MinimumDistance)

	// grid
	grid := LoadGrid(gpxMaster.Waypoints, config.GridSizeInDegree)
	fmt.Printf("Master spatial grid count : %d \n", len(grid))

	gpxFiles, err := GetGPXFiles(config.InputFolder, config.MasterFile, outputFile)
	check(err)
	for _, gpxFile := range gpxFiles {
		gpxAddon, err := ReadGpxFile(config.InputFolder, gpxFile)
		check(err)
		fmt.Printf("GPX Addon from %s: %d waypoints.\n", gpxFile, len(gpxAddon.Waypoints))

		grid.AddGPX(gpxAddon, config.GridSizeInDegree, config.MinimumDistance)
	}
	// flatten the grid
	gpxMaster.Waypoints = make([]gpx.GPXPoint, 0)
	for _, waypoints := range grid {
		gpxMaster.Waypoints = append(gpxMaster.Waypoints, waypoints...)
	}
	// cleanup
	grid.SubstractCloseby(gpxMaster, config.GridSizeInDegree, config.MinimumDistance)

	// augment
	if config.ElevationLookup {
		for i := range gpxMaster.Waypoints {
			if gpxMaster.Waypoints[i].Elevation.Null() {
				CheckElevation(&gpxMaster.Waypoints[i])
			}
		}
	}
	if config.RenderElevation {
		for i := range gpxMaster.Waypoints {
			if gpxMaster.Waypoints[i].Elevation.NotNull() {
				gpxMaster.Waypoints[i].Name += " (" + strconv.FormatFloat(gpxMaster.Waypoints[i].Elevation.Value(), 'f', 0, 64) + " m)"
			}
		}
	}
	//output
	fmt.Printf("GPX Output: %d waypoints to %s \n", len(gpxMaster.Waypoints), outputFile)
	xmlBytes, err := gpxMaster.ToXml(gpx.ToXmlParams{Version: "1.1", Indent: true})
	check(err)
	err = os.WriteFile(outputFile, xmlBytes, 0666)
	check(err)
}

// mapping the REST API call response to data
type ElevationResponse struct {
	Elevation []float64 `json:"elevation"`
}

// resolves the elevation for a given geo-coordinate, at a rought 90m resolution right now.
// the lookup calls are limited in number, please check wbsite for current limits
func CheckElevation(wp *gpx.GPXPoint) error {
	response, err := http.Get("https://api.open-meteo.com/v1/elevation?latitude=" + fmt.Sprintf("%f", wp.Point.Latitude) + "&longitude=" + fmt.Sprintf("%f", wp.Point.Longitude))
	if err == nil {
		responseData, err := io.ReadAll(response.Body)
		if err == nil {
			var elevation ElevationResponse
			json.Unmarshal(responseData, &elevation)
			if len(elevation.Elevation) > 0 {
				wp.Elevation.SetValue(elevation.Elevation[0])
			}
		}
	}
	return err
}
