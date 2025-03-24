package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"regexp"
	"slices"
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

type GpxNameParseData struct {
	Ele       float64
	Countries string
	Name      string
}

// kown patterns for sites offering GPX POI downloads
// TODO: refactor out into a config file
var patterns = map[string]*regexp.Regexp{
	"paesse.info":  regexp.MustCompile(`^(?<Ele>0*\d+) - (?<Countries>[A-Z-]+) - (?<Name>.+)$`),
	"paesse.info/": regexp.MustCompile(`^(?<Ele>\d+) - (?<Countries>[A-Z-]+) - (?<Name>.+)$`),
	"other":        regexp.MustCompile(`^(?<Name>.+)$`),
}

// parse the name into regex groups where possible, decompose groups into data
func parseGpxName(text string) (*GpxNameParseData, error) {
	for _, re := range patterns {
		match := re.FindStringSubmatch(text)
		if match != nil {
			result := &GpxNameParseData{}
			groups := make(map[string]string)
			for i, name := range re.SubexpNames() {
				if i != 0 && name != "" {
					groups[name] = match[i]
				}
			}
			result.Countries = groups["Countries"]
			result.Name = groups["Name"]
			ele, err := strconv.ParseFloat(groups["Ele"], 64)
			result.Ele = ele
			return result, err
		}
	}
	return nil, fmt.Errorf("no matching pattern found for: %s", text)
}

// read a GPX file. parse the name of waypoint for known pattern, to derive elevetion data from it and make names a bit more consistent
func readGpxFile(filename string) (*gpx.GPX, error) {
	var gpxFile *gpx.GPX
	payloadMaster, err := os.ReadFile(filename)
	if err == nil {
		gpxFile, err = gpx.ParseBytes(payloadMaster)
	}
	for i := range gpxFile.Waypoints {
		nameParseData, _ := parseGpxName(gpxFile.Waypoints[i].Name)
		if nameParseData != nil {
			gpxFile.Waypoints[i].Name = nameParseData.Name
			if nameParseData.Ele > 0 {
				gpxFile.Waypoints[i].Elevation.SetValue(nameParseData.Ele)
			}
			if len(nameParseData.Countries) > 0 {
				gpxFile.Waypoints[i].Name += " (" + nameParseData.Countries + ")"
			}
		}
	}
	return gpxFile, err
}

const gridSizeInDegree = float64(0.5)

func main() {
	if len(os.Args) < 5 {
		usage()
	}
	// GPX input files
	gpxFileMaster, err := readGpxFile(os.Args[1])
	check(err)
	gpxFileSlave, err := readGpxFile(os.Args[2])
	check(err)

	maxDistance, err := strconv.ParseFloat(os.Args[4], 64)
	check(err)

	grid := LoadGrid(gpxFileMaster.Waypoints, gridSizeInDegree)
	grid.AddGPX(gpxFileSlave, gridSizeInDegree, maxDistance)

	gpxFileMaster.Waypoints = make([]gpx.GPXPoint, 0)
	for _, waypoints := range grid {
		for _, wp := range waypoints {
			if wp.Elevation.Null() {
				CheckElevation(&wp)
			}
			gpxFileMaster.Waypoints = append(gpxFileMaster.Waypoints, wp)
		}
	}

	grid.SubstractCloseby(gpxFileMaster, gridSizeInDegree, maxDistance)

	// write results
	// TODO file handling with default to master
	// TODO automated split into regions?
	xmlBytes, err := gpxFileMaster.ToXml(gpx.ToXmlParams{Version: "1.1", Indent: true})
	check(err)
	filenameOutMaster := os.Args[3]
	err = os.WriteFile(filenameOutMaster, xmlBytes, 0666)
	check(err)
}

// spatial grid
type WaypointGrid map[string][]gpx.GPXPoint

// compute a key for the grids, derive from grid resolution
func GridKey(wp gpx.GPXPoint, gridSizeInDegree float64) string {
	return fmt.Sprintf("%.2f,%.2f", math.Floor(wp.Latitude/gridSizeInDegree), math.Floor(wp.Longitude/gridSizeInDegree))
}

// load a spatial grid. this limits the number of distance checks to be made.
// TODO: overlay grid for overlap points
func LoadGrid(waypoints []gpx.GPXPoint, gridSizeInDegree float64) WaypointGrid {
	grid := make(WaypointGrid)
	for _, wp := range waypoints {
		key := GridKey(wp, gridSizeInDegree)
		grid[key] = append(grid[key], wp)
	}
	return grid
}

// add  point to a grid cell, but only if not close distance to another
// to avoid closeby to a waypoiint in a neighboring cell, those must be looked into as well
func (grid WaypointGrid) AddGPX(gpxFileSlave *gpx.GPX, gridSizeInDegree float64, maxDistance float64) {
	for _, slaveWaypoint := range gpxFileSlave.Waypoints {
		key := GridKey(slaveWaypoint, gridSizeInDegree)
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
}

func (grid WaypointGrid) SubstractCloseby(gpxFile *gpx.GPX, gridSizeInDegree float64, maxDistance float64) {
	deleteIndex := make([]int, 0)
	for i := range gpxFile.Waypoints {
		for _, key := range NeighboringGridKeys(gpxFile.Waypoints[i], gridSizeInDegree, maxDistance) {
			gridWpIsCloseby := false
			for _, gridWaypoint := range grid[key] {
				if !(gpxFile.Waypoints[i].Name == gridWaypoint.Name && gpxFile.Waypoints[i].Latitude == gridWaypoint.Latitude && gpxFile.Waypoints[i].Longitude == gridWaypoint.Longitude) {
					if gpxFile.Waypoints[i].Distance2D(&gridWaypoint) < maxDistance {
						gridWpIsCloseby = true
						break
					}
				}
			}
			if gridWpIsCloseby {
				deleteIndex = append(deleteIndex, i) //				remove gpxFile.Waypoints[i] from list
			}
		}
	}
	slices.Sort(deleteIndex)
	for _, index := range deleteIndex {
		gpxFile.Waypoints = append(gpxFile.Waypoints[:index], gpxFile.Waypoints[index+1:]...)
	}
}

const metersPerDegree = float64(111000.0)

func NeighboringGridKeys(wp gpx.GPXPoint, gridSizeInDegree float64, maxDistance float64) []string {
	result := make([]string, 0)
	origLat := wp.Latitude
	origLon := wp.Longitude
	diff := maxDistance / metersPerDegree

	for x := -1; x <= 1; x++ {
		for y := -1; y <= 1; y++ {
			wp.Latitude = origLat + (float64(x) * diff)
			wp.Longitude = origLon + (float64(y) * diff)
			key := GridKey(wp, gridSizeInDegree)
			if !slices.Contains(result, key) {
				result = append(result, key)
			}
		}
	}
	return result
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
