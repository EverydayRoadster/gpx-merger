package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"

	"github.com/tkrajina/gpxgo/gpx"
)

func usage() {
	fmt.Println("Merge two GPX files into one, but add points only when they are not close. Augment with rough elevation when missing.")
	fmt.Println("Usage: go run . master.gpx addon.gpx newmaster.gpx mindistance[m]")
	os.Exit(0)
}

func check(e error) {
	if e != nil {
		fmt.Println(e.Error())
		panic(e)
	}
}

// const metersPerDegree = 111000.0 // Approx. 111 km per degree of latitude
const gridSizeInDegree = 0.50

type WaypointGrid map[string][]gpx.GPXPoint

func GridKey(wp gpx.GPXPoint) string {
	return fmt.Sprintf("%.2f,%.2f", math.Floor(wp.Latitude/gridSizeInDegree), math.Floor(wp.Longitude/gridSizeInDegree))
}

func LoadGrid(waypoints []gpx.GPXPoint) WaypointGrid {
	grid := make(WaypointGrid)
	for _, wp := range waypoints {
		key := GridKey(wp)
		grid[key] = append(grid[key], wp)
	}
	return grid
}

func main() {
	if len(os.Args) < 5 {
		usage()
	}
	// GPX input files
	gpxFileMaster, err := readGpxFile(os.Args[1])
	check(err)
	gpxFileSlave, err := readGpxFile(os.Args[2])
	check(err)

	grid := LoadGrid(gpxFileMaster.Waypoints)

	maxDistance, err := strconv.ParseFloat(os.Args[4], 64)
	check(err)

	for _, slaveWaypoint := range gpxFileSlave.Waypoints {
		key := GridKey(slaveWaypoint)
		slaveWpIsCloseby := false
		for _, gridWaypoint := range grid[key] {
			if slaveWaypoint.Distance2D(&gridWaypoint) < maxDistance {
				slaveWpIsCloseby = true
				break
			}
		}
		if !slaveWpIsCloseby {
			grid[key] = append(grid[key], slaveWaypoint)
		}
	}

	gpxFileMaster.Waypoints = make([]gpx.GPXPoint, 0)

	for _, waypoints := range grid {
		for _, wp := range waypoints {
			if wp.Elevation.Null() {
				CheckElevation(&wp)
			}
			gpxFileMaster.Waypoints = append(gpxFileMaster.Waypoints, wp)
		}
	}

	xmlBytes, err := gpxFileMaster.ToXml(gpx.ToXmlParams{Version: "1.1", Indent: true})
	check(err)
	filenameOutMaster := os.Args[3]
	err = os.WriteFile(filenameOutMaster, xmlBytes, 0666)
	check(err)
}

func readGpxFile(filename string) (*gpx.GPX, error) {
	var gpxFile *gpx.GPX
	payloadMaster, err := os.ReadFile(filename)
	if err == nil {
		gpxFile, err = gpx.ParseBytes(payloadMaster)
	}
	return gpxFile, err
}

type ElevationResponse struct {
	Elevation []float64 `json:"elevation"`
}

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
