package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/chromedp/chromedp"
)

type Symbol struct {
	Name string
	Path string
	Kind string
	File string
}

func main() {

	known404, err := readFileLines("./404")
	if err != nil {
		log.Fatal(err)
	}

	targetDir := "./cache/meta"
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		log.Fatal(err)
	}
	toTargetPath := func(p string) string {
		return strings.Replace(p, "symbols", targetDir, 1)
	}

	ctx := context.Background()
	ctx, cancel := chromedp.NewRemoteAllocator(context.Background(), "http://localhost:9222/devtools/browser")
	defer cancel()
	ctx, cancel = chromedp.NewContext(ctx)
	defer cancel()

	ch := make(chan Symbol, 1024)

	go func() {
		err := filepath.Walk("./symbols", func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if _, err := os.Stat(toTargetPath(path)); !os.IsNotExist(err) {
				return nil
			}

			// Process only regular files with ".json" extension
			if !info.IsDir() && filepath.Ext(path) == ".json" {
				data, err := ioutil.ReadFile(path)
				if err != nil {
					return err
				}

				var symbol Symbol
				if err := json.Unmarshal(data, &symbol); err != nil {
					return err
				}
				symbol.File = path

				ch <- symbol
			}

			return nil
		})
		if err != nil {
			log.Fatal(err)
		}
		close(ch)
	}()

	for sym := range ch {

		if strIn(known404, sym.Path) {
			//fmt.Println("Skipping known 404")
			continue
		}

		var pageText string
		resp, err := chromedp.RunResponse(ctx,
			chromedp.Navigate(fmt.Sprintf("https://developer.apple.com/tutorials/data/documentation/%s.json?language=objc", sym.Path)),
			chromedp.Text("body", &pageText, chromedp.NodeVisible, chromedp.ByQuery))
		if err != nil || resp == nil {
			fmt.Println(sym.Path, " => ", err)
			continue
		}
		if resp.Status == http.StatusOK {
			var d any
			if err := json.Unmarshal([]byte(pageText), &d); err != nil {
				fmt.Println(pageText)
				log.Fatal(err)
			}

			if err := os.MkdirAll(filepath.Dir(toTargetPath(sym.File)), 0755); err != nil {
				log.Fatal(err)
			}
			if err := ioutil.WriteFile(toTargetPath(sym.File), []byte(pageText), 0644); err != nil {
				log.Fatal(err)
			}

			fmt.Println(sym.Path, " => ", toTargetPath(sym.File))
		} else {
			fmt.Println(sym.Path, " => ", resp.Status)
		}

	}

}

func strIn(slice []string, str string) bool {
	for _, s := range slice {
		if strings.HasPrefix(str, s) {
			return true
		}
	}
	return false
}

func readFileLines(filename string) ([]string, error) {
	var lines []string
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}
