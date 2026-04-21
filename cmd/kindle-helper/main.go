package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kaikozlov/kindle-koplugin/internal/drm"
	"github.com/kaikozlov/kindle-koplugin/internal/jsonout"
	"github.com/kaikozlov/kindle-koplugin/internal/kfx"
	"github.com/kaikozlov/kindle-koplugin/internal/scan"
)

const version = 1

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: kindle-helper <scan|convert|cover|drm-init|decrypt|position> [flags]\n")
		os.Exit(2)
	}

	switch os.Args[1] {
	case "scan":
		cmdScan(os.Args[2:])
	case "convert":
		cmdConvert(os.Args[2:])
	case "cover":
		cmdCover(os.Args[2:])
	case "drm-init":
		cmdDrmInit(os.Args[2:])
	case "decrypt":
		cmdDecrypt(os.Args[2:])
	case "position":
		cmdPosition(os.Args[2:])
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

func cmdDecrypt(args []string) {
	fs := flag.NewFlagSet("decrypt", flag.ExitOnError)
	input := fs.String("input", "", "input DRMION KFX file path")
	output := fs.String("output", "", "output KFX-zip file path")
	cacheDir := fs.String("cache-dir", "", "cache directory for drm_keys.json")
	fs.Parse(args)

	if *input == "" {
		fmt.Fprintf(os.Stderr, "decrypt: --input is required\n")
		os.Exit(2)
	}
	if *output == "" {
		fmt.Fprintf(os.Stderr, "decrypt: --output is required\n")
		os.Exit(2)
	}

	data, err := os.ReadFile(*input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "decrypt: %v\n", err)
		os.Exit(1)
	}
	if !bytes.HasPrefix(data, kfx.DRMIONSignature()) {
		fmt.Fprintf(os.Stderr, "decrypt: not a DRMION file\n")
		os.Exit(1)
	}

	pageKey, err := kfx.FindPageKey(*input, *cacheDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "decrypt: find page key: %v\n", err)
		os.Exit(1)
	}

	contData, err := kfx.DecryptDRMION(data, pageKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "decrypt: %v\n", err)
		os.Exit(1)
	}
	log.Printf("decrypted main container: %d bytes", len(contData))

	// Collect sidecar CONT and DRMION blobs from .sdr/assets/
	sidecarRoot := strings.TrimSuffix(*input, filepath.Ext(*input)) + ".sdr"
	contBlobs, drmionBlobs := collectSidecarBlobs(sidecarRoot)

	// Decrypt DRMION sidecar blobs
	var allBlobs []blobEntry
	allBlobs = append(allBlobs, blobEntry{name: "main.kfx", data: contData})
	for _, b := range contBlobs {
		allBlobs = append(allBlobs, b)
	}
	for _, b := range drmionBlobs {
		dec, err := kfx.DecryptDRMION(b.data, pageKey)
		if err != nil {
			log.Printf("skipping DRMION sidecar %s: %v", b.name, err)
			continue
		}
		allBlobs = append(allBlobs, blobEntry{name: b.name, data: dec})
		log.Printf("decrypted sidecar %s: %d bytes", b.name, len(dec))
	}

	// Write as KFX-zip
	if err := writeKFXZip(*output, allBlobs); err != nil {
		fmt.Fprintf(os.Stderr, "decrypt: write: %v\n", err)
		os.Exit(1)
	}
	log.Printf("wrote %s with %d entries", *output, len(allBlobs))
}

type blobEntry struct {
	name string
	data []byte
}

func collectSidecarBlobs(root string) (cont []blobEntry, drmion []blobEntry) {
	var names []string
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		names = append(names, path)
		return nil
	})
	sort.Strings(names)

	for _, name := range names {
		data, err := os.ReadFile(name)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			rel = filepath.Base(name)
		}
		switch {
		case len(data) >= 4 && data[0] == 'C' && data[1] == 'O' && data[2] == 'N' && data[3] == 'T':
			cont = append(cont, blobEntry{name: rel, data: data})
		case bytes.HasPrefix(data, kfx.DRMIONSignature()):
			drmion = append(drmion, blobEntry{name: rel, data: data})
		}
	}
	return
}

func writeKFXZip(outputPath string, entries []blobEntry) error {
	os.MkdirAll(filepath.Dir(outputPath), 0755)
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := zip.NewWriter(f)
	defer w.Close()

	for _, e := range entries {
		fw, err := w.Create(e.name)
		if err != nil {
			return err
		}
		if _, err := fw.Write(e.data); err != nil {
			return err
		}
	}
	return nil
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

func cmdCover(args []string) {
	fs := flag.NewFlagSet("cover", flag.ExitOnError)
	sdrDir := fs.String("sdr-dir", "", "path to the .sdr sidecar directory")
	output := fs.String("output", "", "output JPEG file path (writes to stdout if empty)")
	fs.Parse(args)

	if *sdrDir == "" {
		fmt.Fprintf(os.Stderr, "cover: --sdr-dir is required\n")
		os.Exit(2)
	}

	jpeg := kfx.ExtractCoverJPEG(*sdrDir)
	if jpeg == nil {
		writeJSON(jsonout.CoverResult{
			Version: version,
			OK:      false,
			Message: "no cover image found in metadata.kfx",
		})
		return
	}

	if *output != "" {
		if err := os.WriteFile(*output, jpeg, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "cover: failed to write output: %v\n", err)
			os.Exit(1)
		}
		writeJSON(jsonout.CoverResult{
			Version: version,
			OK:      true,
			Size:    len(jpeg),
		})
	} else {
		os.Stdout.Write(jpeg)
	}
}
