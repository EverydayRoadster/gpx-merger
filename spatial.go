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

func WaypointCompare(left gpx.GPXPoint, right gpx.GPXPoint) bool {
	return left.Name == right.Name && left.Latitude == right.Latitude && left.Longitude == right.Longitude && left.Elevation == right.Elevation
}

// eliminate those close by waypoints
func (grid WaypointGrid) SubstractCloseby(gpxFile *gpx.GPX, gridSizeInDegree float64, minDistance float64) {
	result := make([]gpx.GPXPoint, 0)
	smallestDistance := math.MaxFloat64
	smallestDistanceBetween := []string{}

	for i := range gpxFile.Waypoints {
		gridWpIsCloseby := false
		for _, key := range NeighboringGridKeys(gpxFile.Waypoints[i], gridSizeInDegree, minDistance) {
			if gridWpIsCloseby {
				break
			}
			for _, gridWaypoint := range grid[key] {
				distance := gpxFile.Waypoints[i].Distance2D(&gridWaypoint)
				if distance < minDistance {
					// do not remove if the same waypoint from the two lists
					if !WaypointCompare(gpxFile.Waypoints[i], gridWaypoint) {
						// mark for not include here
						gridWpIsCloseby = true
						fmt.Printf("Waypoint %s is marked for deletion, as it is too close to %s, \n", gpxFile.Waypoints[i].Name, gridWaypoint.Name)
						// take out from grid list
						gridDeleteKey := GridKey(gpxFile.Waypoints[i], gridSizeInDegree)
						for k := range grid[gridDeleteKey] {
							if WaypointCompare(grid[gridDeleteKey][k], gpxFile.Waypoints[i]) {
								grid[gridDeleteKey] = slices.Delete(grid[gridDeleteKey], k, k+1)
								break
							}
						}
						break
					}
				} else {
					if smallestDistance > distance {
						smallestDistance = distance
						smallestDistanceBetween = []string{gpxFile.Waypoints[i].Name, gridWaypoint.Name}
					}
				}
			}
		}
		if !gridWpIsCloseby {
			result = append(result, gpxFile.Waypoints[i])
		}
	}
	gpxFile.Waypoints = result
	fmt.Printf("Smallest distance between waypoints is %.2f m from %s to %s \n", smallestDistance, smallestDistanceBetween[0], smallestDistanceBetween[1])
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
