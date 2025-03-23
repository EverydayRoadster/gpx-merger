package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"regexp"
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
var patterns = map[string]*regexp.Regexp{
	"paesse.info": regexp.MustCompile(`^(?P<Ele>\d+) - (?P<Countries>[A-Z-]+) - (?P<Name>.+)$`),
	"other":       regexp.MustCompile(`^(?P<Name>.+)$`),
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

// grid resolution
const gridSizeInDegree = 0.50

// spatial grid
type WaypointGrid map[string][]gpx.GPXPoint

// compute a key for the grids, derive from grid resolution
func GridKey(wp gpx.GPXPoint) string {
	return fmt.Sprintf("%.2f,%.2f", math.Floor(wp.Latitude/gridSizeInDegree), math.Floor(wp.Longitude/gridSizeInDegree))
}

// load a spatial grid. this limits the number of distance checks to be made.
// TODO: overlay grid for overlap points
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
