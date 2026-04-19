package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kaikozlov/kindle-koplugin/internal/drm"
	"github.com/kaikozlov/kindle-koplugin/internal/jsonout"
	"github.com/kaikozlov/kindle-koplugin/internal/kfx"
	"github.com/kaikozlov/kindle-koplugin/internal/scan"
)

const version = 1

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: kindle-helper <scan|convert|drm-init> [flags]\n")
		os.Exit(2)
	}

	switch os.Args[1] {
	case "scan":
		cmdScan(os.Args[2:])
	case "convert":
		cmdConvert(os.Args[2:])
	case "drm-init":
		cmdDrmInit(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(2)
	}
}

func cmdScan(args []string) {
	fs := flag.NewFlagSet("scan", flag.ExitOnError)
	root := fs.String("root", "", "root directory to scan")
	fs.Parse(args)

	if *root == "" {
		fmt.Fprintf(os.Stderr, "scan: --root is required\n")
		os.Exit(2)
	}

	result, err := scan.Run(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scan: %v\n", err)
		writeJSON(jsonout.ScanResult{
			Version: version,
			Root:    *root,
			Books:   []jsonout.ScanBook{},
		})
		os.Exit(1)
	}

	result.Version = version
	writeJSON(result)
}

func cmdConvert(args []string) {
	fs := flag.NewFlagSet("convert", flag.ExitOnError)
	input := fs.String("input", "", "input KFX file path")
	output := fs.String("output", "", "output EPUB file path")
	cacheDir := fs.String("cache-dir", "", "cache directory for drm_keys.json")
	fs.Parse(args)

	if *input == "" {
		fmt.Fprintf(os.Stderr, "convert: --input is required\n")
		os.Exit(2)
	}
	if *output == "" {
		fmt.Fprintf(os.Stderr, "convert: --output is required\n")
		os.Exit(2)
	}

	err := kfx.ConvertFile(*input, *output, *cacheDir)
	if err == nil {
		writeJSON(jsonout.ConvertResult{
			Version:    version,
			OK:         true,
			OutputPath: *output,
		})
		return
	}

	code := "error"
	message := err.Error()

	var drmErr *kfx.DRMError
	if errors.As(err, &drmErr) {
		code = "drm"
	}

	var unsupportedErr *kfx.UnsupportedError
	if errors.As(err, &unsupportedErr) {
		code = "unsupported"
	}

	writeJSON(jsonout.ConvertResult{
		Version: version,
		OK:      false,
		Code:    code,
		Message: message,
	})
}

func cmdDrmInit(args []string) {
	fs := flag.NewFlagSet("drm-init", flag.ExitOnError)
	root := fs.String("root", "/mnt/us/documents", "root directory to scan for DRM books")
	cacheDir := fs.String("cache-dir", "", "cache directory for drm_keys.json")
	fs.Parse(args)

	result, err := drm.Run(*root, getPluginDir(), *cacheDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "drm-init: %v\n", err)
		writeJSON(jsonout.DrmInitResult{
			Version: version,
			OK:      false,
			Message: err.Error(),
		})
		os.Exit(1)
	}

	writeJSON(jsonout.DrmInitResult{
		Version:    version,
		OK:         true,
		BooksFound: result.BooksFound,
		KeysFound:  result.KeysFound,
	})
}

func writeJSON(v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "json marshal error: %v\n", err)
		os.Exit(1)
	}
	os.Stdout.Write(data)
	os.Stdout.Write([]byte("\n"))
}

func getPluginDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}
