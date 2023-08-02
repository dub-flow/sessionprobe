package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net/http"
	"io"
	neturl "net/url" 
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"path/filepath"

	"github.com/fatih/color"
)

// global counter for URLs processed
var counter int32

var (
    green  = color.New(color.FgGreen).SprintFunc()
    red = color.New(color.FgRed).SprintFunc()
)

func Info(format string, a ...interface{}) {
	log.Printf("%s", green(fmt.Sprintf(format, a...)))
}

func Error(format string, a ...interface{}) {
	log.Printf("%s", red(fmt.Sprintf(format, a...)))
}

func main() {
	headersPtr := flag.String("headers", "", "HTTP headers to be used in the requests in the format \"Key1:Value1;Key2:Value2;...\"")
	urlsPtr := flag.String("urls", "", "File containing the URLs to be checked")
	threadPtr := flag.Int("threads", 10, "Number of threads (default: 10)")
	outPtr := flag.String("out", "output.txt", "Output file (default: output.txt)")
	proxyPtr := flag.String("proxy", "", "Proxy URL (default: \"\")")
	flag.Parse()

	color.Green("##################################\n")
	color.Green("#                                #\n")
	color.Green("#          SessionProbe          #\n")
	color.Green("#                                #\n")
	color.Green("##################################\n\n")

	// check if the AppVersion was already set during compilation - otherwise manually get it from `./VERSION`
	CheckAppVersion()
	color.Yellow("Current version: %s\n\n", AppVersion)

	// the `urls` flag is required
	if *urlsPtr == "" {
		Error("Please provide a urls file using the -urls argument")
		return
	}

	var headers map[string]string
	if *headersPtr != "" {
		headers = parseHeaders(*headersPtr)
	}


	file, err := os.Open(*urlsPtr)
	if err != nil {
		Error("%s", err)
		return
	}
	defer file.Close()

	outFile, err := os.Create(*outPtr)
	if err != nil {
		Error("%s", err)
		return
	}
	defer outFile.Close()

	// create semaphore with the specified number of threads
	sem := make(chan bool, *threadPtr)
	// make sure to wait for all threads to finish before exiting the program
	var wg sync.WaitGroup

	// read the URLs line by line
	scanner := bufio.NewScanner(file)

	// using a map to deduplicate URLs (since map keys are unique in Go, so by using the URLs as the keys, 
	// we effectively remove any duplicate URLs)
	urls := make(map[string]bool)
	for scanner.Scan() {
		url := scanner.Text()
		urls[url] = true
	}

	if scanner.Err() != nil {
		Error("%s", scanner.Err())
	}

	// total number of URLs
	urlCount := len(urls) 

	Info("Starting to check %d URLs with %d threads", urlCount, *threadPtr)

	// map to store URLs by status code
	urlStatuses := make(map[int][]string)
	var urlStatusesMutex sync.Mutex

	// process each URL in the deduplicated map
	for url := range urls {
		wg.Add(1)

		// will block if there is already `*threadPtr` threads running
		sem <- true

		// launch a new goroutine for each URL
		go func(url string) {
			defer wg.Done()
			statusCode, length := checkURL(url, headers, *proxyPtr)
			// add URL to status code map
			urlStatusesMutex.Lock()
			urlStatuses[statusCode] = append(urlStatuses[statusCode], fmt.Sprintf("%s => Length: %d", url, length))
			urlStatusesMutex.Unlock()

			// removing a slot in the semaphore so that a new goroutine can be created
			<-sem

			// increment the global counter
			atomic.AddInt32(&counter, 1)
			Info("Progress: %.2f%%", float64(counter)/float64(urlCount)*100)
		}(url)
	}

	// wait for all threads to finish
	wg.Wait()

	// get the list of status codes
	statusCodes := make([]int, 0, len(urlStatuses))
	for statusCode := range urlStatuses {
		statusCodes = append(statusCodes, statusCode)
	}
	// sort the status codes
	sort.Ints(statusCodes)

	// write output to file, sorted by status code
	for _, statusCode := range statusCodes {
		_, err = outFile.WriteString(fmt.Sprintf("Responses with Status Code: %d\n\n", statusCode))
		if err != nil {
			Error("%s", err)
		}

		// Sort URLs by extension for each status code
		sort.Slice(urlStatuses[statusCode], func(i, j int) bool {
			return filepath.Ext(urlStatuses[statusCode][i]) < filepath.Ext(urlStatuses[statusCode][j])
		})

		for _, url := range urlStatuses[statusCode] {
			_, err = outFile.WriteString(fmt.Sprintf("%s\n", url))
			if err != nil {
				Error("%s", err)
			}
		}
		_, err = outFile.WriteString("\n")
		if err != nil {
			Error("%s", err)
		}
	}
}

func parseHeaders(headers string) map[string]string {
	headerMap := make(map[string]string)
	pairs := strings.Split(headers, ";")

	for _, pair := range pairs {
		parts := strings.SplitN(pair, ":", 2)

		if len(parts) != 2 {
			Error("Invalid header format: %s", pair)
			continue
		}
		
		headerMap[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}

	return headerMap
}

// function to do the HTTP request and check the response's status code and response length
func checkURL(url string, headers map[string]string, proxy string) (int, int64) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		Error("%s", err)
		return 0, 0
	}

	for key, value := range headers {
		req.Header.Add(key, value)
	}

	var client *http.Client

	if proxy != "" {
		// if a proxy was provided via `-proxy`
		proxyURL, _ := neturl.Parse(proxy)
		client = &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			},
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	} else {
		// if no proxy was provided
		client = &http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		Error("Failed to make the request: %s", err)
		return 0, 0
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		Error("Failed to read the response: %s", err)
		return resp.StatusCode, 0
	}

	return resp.StatusCode, int64(len(body))
}
