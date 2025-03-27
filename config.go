package main

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

type Config struct {
	MinimumDistance  float64           `yaml:"minimumDistance"`
	GridSizeInDegree float64           `yaml:"gridSizeInDegree"`
	MasterFile       string            `yaml:"master"`
	InputFolder      string            `yaml:"inputFolder"`
	Files            map[string]string `yaml:"files"`
	ElevationLookup  bool              `yaml:"elevationLookup"`
	RenderElevation  bool              `yaml:"renderElevation"`
}

var config Config
var patterns map[string]*regexp.Regexp
var err error

const metersPerDegree = float64(111000.0)

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
