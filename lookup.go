package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

func main() {
	path := os.Args[1]

	r, err := zip.OpenReader("./symbols.zip")
	if err != nil {
		log.Fatal(err)
	}
	defer r.Close()

	var matchingFiles []*zip.File
	for _, file := range r.File {
		filename := strings.TrimPrefix(file.Name, "symbols/")
		if filename == fmt.Sprintf("%s.json", path) {
			matchingFiles = append(matchingFiles, file)
		}
		if strings.HasPrefix(filename, path+"/") && filename[len(filename)-1] != '/' {
			matchingFiles = append(matchingFiles, file)
		}
	}

	var result []any
	for i := len(matchingFiles) - 1; i >= 0; i-- {
		d, err := loadData[map[string]any](matchingFiles[i])
		if err != nil {
			log.Fatal(err)
		}
		result = append(result, d)
	}

	b, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(b))
}

func loadData[T any](file *zip.File) (v T, err error) {
	var reader io.ReadCloser
	reader, err = file.Open()
	if err != nil {
		return v, err
	}
	defer reader.Close()

	b, err := io.ReadAll(reader)
	if err != nil {
		return
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return v, err
	}
	return
}
