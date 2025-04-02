# gpx-merger

Merges waypoints from multile gpx files into one, and removes any duplicate waypoints from the new gpx file. 

Duplicate waypoints are judged by to be located within a configurable distance.

Along the way, gpx-merger may clean up names of waypoints into a consistent appearance and may add elevation data to waypoints, where it is missing.

Syntax: 
`gpx-merger <config.yaml> <output.gpx>`

## Use case:
Collecting waypoints from different sources into your roadtrip planner, this often creates duplicate entries on your map. 

Manually editing out those duplicates is not only time consuming and prone to error, it also will hurt every time one of the source files is re-imported for updates.

gpx-merger unitfies all gpx waypoint files from a single directory into one waypoint gpx file, which you may import into your road trip planner as a single source. 

With consolidation to happen outside and before the planner tool, this shields from any unwanted manual labor.

## config.yaml

The config controls the merge process. To do so, it defines the following elements in a yaml structure:

**master**: "EverydayRoadster.gpx"

A master gpx file is the baseline, typically some reliable source, e.g. your own gpx file. It may contain zero or many waypoints.

**inputFolder**: "../GettingHigh"

The folder from where to read all other gpx files from. If the master gpx file or the output gpx file (command line parameter) is located in this same folder, those files will be ignored for processing.

**files**:

  Paesse-Info-Tarred-Passes.gpx: "^(?P<Ele>0*\\d+)\\s-\\s(?P<Countries>[A-Z-]+?)\\s-\\s(?P<Name>.+)$"  

  Paesse-Info-Tarred-Mountain-Roads-Dead-End.gpx: "^(?P<Ele>0*\\d+)\\s-\\s(?P<Countries>[A-Z-]+?)\\s-\\s(?P<Name>.+)$"  

  alpenrouten.gpx: "^(?P<Name>.+), (?P<Prenom>.+)$"

  poi_export.gpx: "^(?P<Name>.+)$"

  default: "^(?P<Name>.+)$"

The default regexp must be present at least.

For a gpx input file by the name given, it applies a regular expression on the name elements of input waypoints, to detect elemets:

- Elevation
- Country
- Name
- Prenom

The elements will then be used to construct the new waypoint in a more consistent format, where possible.

**minimumDistance**: 480

The minimum distance between two wayoints in meters, to qualify them as identical. Right now the distance only is the only qualification criteria.

The printout of the tool states names of the closest distinct waypoints. This may give hint to a distance well chosen.

**gridSizeInDegree**: 0.5

The fence length in degree (approximated) for each spatial unit waypoint ar being collected in. Relevant for performance tuning only. 

**elevationLookup**: false

If a online lookup should be performed for elevation of a waypoint, in case missing. Setting to true will increase processing time. The nnumber of lookups allowed is limited on a daily basis, check https://open-meteo.com for details.

**renderElevation**: true

If the elevation data should become part of the waypoint name (202 m).
