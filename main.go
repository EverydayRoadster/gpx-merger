package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/tkrajina/gpxgo/gpx"
	"gopkg.in/yaml.v3"
)

func usage() {
	fmt.Println("Merge GPX files into one, starting off from a master GPX file. But add points only when they are not close to others. Augment point with rough elevation data in case missing. Config data is read from a file, too.")
	fmt.Println("Usage: go run . master.gpx")
	os.Exit(0)
}

func check(e error) {
	if e != nil {
		fmt.Println(e.Error())
		panic(e)
	}
}

type Config struct {
	MinimumDistance  float64           `yaml:"minimumDistance"`
	GridSizeInDegree float64           `yaml:"gridSizeInDegree"`
	MasterFile       string            `yaml:"master"`
	Files            map[string]string `yaml:"files"`
	InputFolder      string            `yaml:"inputFolder"`
	OutputFolder     string            `yaml:"outputFolder"`
	ElevationLookup  bool              `yaml:"elevationLookup"`
	RenderElevation  bool              `yaml:"renderElevation"`
	Regexp           map[string]regexp.Regexp
}

var config Config
var patterns map[string]*regexp.Regexp
var err error

// load the config
func loadConfig(configFileName string) error {
	configFile, err := os.ReadFile(configFileName)
	check(err)

	err = yaml.Unmarshal(configFile, &config)
	check(err)

	// already compile regex patterns into global var
	patterns = make(map[string]*regexp.Regexp)
	for key, pattern := range config.Files {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("invalid regex for %s: %v", key, err)
		}
		patterns[key] = re
	}
	return nil
}

type GpxNameParseData struct {
	Ele       float64
	Countries string
	Name      string
	Prenom    string
}

// parse the name into regex groups where possible, decompose groups into data
func parseGpxName(filename string, text string) (*GpxNameParseData, error) {
	re := patterns[filename]
	if re == nil {
		re = patterns["default"]
	}
	if re != nil {
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
			result.Prenom = groups["Prenom"]
			if len(groups["Ele"]) > 0 {
				ele, _ := strconv.ParseFloat(groups["Ele"], 64)
				result.Ele = ele
			}
			return result, err
		}
	}
	return nil, fmt.Errorf("no matching pattern found for: %s", text)
}

// read a GPX file. parse the name of waypoint for known pattern, to derive elevetion data from it and make names a bit more consistent
func readGpxFile(inputDir string, filename string) (*gpx.GPX, error) {
	var gpxFile *gpx.GPX
	payload, err := os.ReadFile(inputDir + filename)
	if err == nil {
		gpxFile, err = gpx.ParseBytes(payload)
	}
	check(err)
	for i := range gpxFile.Waypoints {
		// in case of elevation data missing, this is worth parsing for that
		nameParseData, _ := parseGpxName(filename, gpxFile.Waypoints[i].Name)
		if nameParseData != nil {
			gpxFile.Waypoints[i].Name = strings.Trim(nameParseData.Prenom+" "+nameParseData.Name, " ")
			if gpxFile.Waypoints[i].Elevation.Null() && nameParseData.Ele > 0 {
				gpxFile.Waypoints[i].Elevation.SetValue(nameParseData.Ele)
			}
		}
	}

	return gpxFile, err
}

// get a list of GPX files from input dir, excluding master and output file, in case those would be found in the same dir
func GetGPXFiles(inputDir string, master string, output string) ([]string, error) {
	inputDir = strings.TrimRight(inputDir, " /") + "/"
	var gpxFiles []string
	err := filepath.WalkDir(inputDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && filepath.Ext(d.Name()) == ".gpx" && path != master && path != output {
			gpxFiles = append(gpxFiles, d.Name())
		}
		return nil
	})
	return gpxFiles, err
}

const metersPerDegree = float64(111000.0)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	err = loadConfig("gpx-merger.yaml")
	check(err)

	// GPX input files
	outputFile := os.Args[1]

	gpxMaster, err := readGpxFile("", config.MasterFile)
	check(err)
	fmt.Printf("GPX Master: %d waypoints.\n", len(gpxMaster.Waypoints))
	fmt.Printf("Spatial grid width : %f km, using Euclidean distance calculation!\n", config.GridSizeInDegree*metersPerDegree/1000)
	check(err)
	fmt.Printf("Minimum distance between points : %f m.\n", config.MinimumDistance)

	grid := LoadGrid(gpxMaster.Waypoints, config.GridSizeInDegree)
	fmt.Printf("Master spatial grid count : %d \n", len(grid))

	gpxFiles, err := GetGPXFiles(config.InputFolder, config.MasterFile, outputFile)
	check(err)
	for _, gpxFile := range gpxFiles {
		gpxAddon, err := readGpxFile(config.InputFolder, gpxFile)
		check(err)
		fmt.Printf("GPX Addon from %s: %d waypoints.\n", gpxFile, len(gpxAddon.Waypoints))

		grid.AddGPX(gpxAddon, config.GridSizeInDegree, config.MinimumDistance)
		fmt.Printf("+Addon spatial grid count : %d \n", len(grid))
	}
	// flatten the grid
	gpxMaster.Waypoints = make([]gpx.GPXPoint, 0)
	for _, waypoints := range grid {
		gpxMaster.Waypoints = append(gpxMaster.Waypoints, waypoints...)
	}
	// cleanup
	grid.SubstractCloseby(gpxMaster, config.GridSizeInDegree, config.MinimumDistance)

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

	fmt.Printf("GPX Output: %d waypoints.\n", len(gpxMaster.Waypoints))

	// write results
	// TODO file handling with default to master
	// TODO automated split into regions?
	xmlBytes, err := gpxMaster.ToXml(gpx.ToXmlParams{Version: "1.1", Indent: true})
	check(err)
	err = os.WriteFile(outputFile, xmlBytes, 0666)
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
		gridWpIsCloseby := false
		for _, key := range NeighboringGridKeys(gpxFile.Waypoints[i], gridSizeInDegree, maxDistance) {
			if gridWpIsCloseby {
				break
			}
			for _, gridWaypoint := range grid[key] {
				if gpxFile.Waypoints[i].Distance2D(&gridWaypoint) < maxDistance {
					if !(gpxFile.Waypoints[i].Name == gridWaypoint.Name && gpxFile.Waypoints[i].Latitude == gridWaypoint.Latitude && gpxFile.Waypoints[i].Longitude == gridWaypoint.Longitude) {
						gridWpIsCloseby = true
						fmt.Printf("Waypoint %s is marked for deletion, as it is too close to %s, \n", gpxFile.Waypoints[i].Name, gridWaypoint.Name)
						break
					}
				}
			}
		}
		if gridWpIsCloseby {
			deleteIndex = append(deleteIndex, i)
		}
	}
	slices.Sort(deleteIndex)
	for _, index := range deleteIndex {
		gpxFile.Waypoints = append(gpxFile.Waypoints[:index], gpxFile.Waypoints[index+1:]...)
	}
}

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
