package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	dataPath, err := filepath.Abs(dataFileName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve data file: %v\n", err)
		os.Exit(1)
	}

	data, err := loadData(dataPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load planner data: %v\n", err)
		os.Exit(1)
	}

	program := newModel(data, dataPath)
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "run planner: %v\n", err)
		os.Exit(1)
	}
}
