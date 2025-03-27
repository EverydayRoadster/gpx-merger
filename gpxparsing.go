package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tkrajina/gpxgo/gpx"
)

type GpxNameParseData struct {
	Ele       float64
	Countries string
	Name      string
	Prenom    string
}

// parse the name into regex groups where possible, decompose groups into data
func ParseGpxName(filename string, text string) (*GpxNameParseData, error) {
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
func ReadGpxFile(inputDir string, filename string) (*gpx.GPX, error) {
	var gpxFile *gpx.GPX
	payload, err := os.ReadFile(inputDir + filename)
	if err == nil {
		gpxFile, err = gpx.ParseBytes(payload)
	}
	check(err)
	for i := range gpxFile.Waypoints {
		// in case of elevation data missing, this is worth parsing for that
		nameParseData, _ := ParseGpxName(filename, gpxFile.Waypoints[i].Name)
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
