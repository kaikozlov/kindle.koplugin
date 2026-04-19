package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/kaikozlov/kindle-koplugin/internal/jsonout"
	"github.com/kaikozlov/kindle-koplugin/internal/kfx"
	"github.com/kaikozlov/kindle-koplugin/internal/scan"
)

const version = 1

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: kindle-helper <scan|convert> [flags]\n")
		os.Exit(2)
	}

	switch os.Args[1] {
	case "scan":
		cmdScan(os.Args[2:])
	case "convert":
		cmdConvert(os.Args[2:])
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
	fs.Parse(args)

	if *input == "" {
		fmt.Fprintf(os.Stderr, "convert: --input is required\n")
		os.Exit(2)
	}
	if *output == "" {
		fmt.Fprintf(os.Stderr, "convert: --output is required\n")
		os.Exit(2)
	}

	err := kfx.ConvertFile(*input, *output)
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

func writeJSON(v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "json marshal error: %v\n", err)
		os.Exit(1)
	}
	os.Stdout.Write(data)
	os.Stdout.Write([]byte("\n"))
}
