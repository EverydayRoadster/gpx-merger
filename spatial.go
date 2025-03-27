package main

import (
	"fmt"
	"math"
	"slices"

	"github.com/tkrajina/gpxgo/gpx"
)

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
func (grid WaypointGrid) AddGPX(gpxFile *gpx.GPX, gridSizeInDegree float64, maxDistance float64) {
	for _, waypoint := range gpxFile.Waypoints {
		key := GridKey(waypoint, gridSizeInDegree)
		wpIsCloseby := false
		for _, gridWaypoint := range grid[key] {
			if waypoint.Distance2D(&gridWaypoint) < maxDistance {
				wpIsCloseby = true
				break
			}
		}
		if !wpIsCloseby {
			grid[key] = append(grid[key], waypoint)
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
