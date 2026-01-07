package command

import (
	"os"
	"path/filepath"
	"strings"
)

// getBinDirectory returns the bin directory path relative to the working directory
func getBinDirectory() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return wd, nil
}

// updateCommandEnv updates the command environment to include bin directory in PATH
func updateCommandEnv(cmdEnv []string, binDir string) []string {
	absBinPath, err := filepath.Abs(binDir)
	if err != nil {
		return cmdEnv
	}

	currentPath := os.Getenv("PATH")
	if currentPath == "" {
		currentPath = absBinPath
	} else {
		// Check if bin directory is already in PATH
		pathDirs := filepath.SplitList(currentPath)
		for _, dir := range pathDirs {
			if dir == absBinPath {
				// Already in PATH, return original env
				return cmdEnv
			}
		}
		// Prepend bin directory to PATH
		currentPath = absBinPath + string(os.PathListSeparator) + currentPath
	}

	// Update or add PATH in environment
	newEnv := make([]string, 0, len(cmdEnv)+1)
	pathFound := false
	for _, env := range cmdEnv {
		if strings.HasPrefix(env, "PATH=") {
			newEnv = append(newEnv, "PATH="+currentPath)
			pathFound = true
		} else {
			newEnv = append(newEnv, env)
		}
	}
	if !pathFound {
		newEnv = append(newEnv, "PATH="+currentPath)
	}

	return newEnv
}

