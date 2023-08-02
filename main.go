package main

import (
	"bufio"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/fatih/color"
)

// global counter for URLs processed
var counter int32

var (
    Info  = color.New(color.FgGreen).PrintfFunc()
    Error = color.New(color.FgRed).PrintfFunc()
)

// a logger that has a proper timestamp
var (
    InfoLogger  = log.New(os.Stdout, Info("[INFO] "), log.LstdFlags)
    ErrorLogger = log.New(os.Stderr, Error("[ERROR] "), log.LstdFlags)
)

func main() {
	cookiePtr := flag.String("cookie", "", "Session cookie to be used in the requests")
	urlsPtr := flag.String("urls", "", "File containing the URLs to be checked")
	threadPtr := flag.Int("threads", 10, "Number of threads (default: 10)")
	outPtr := flag.String("out", "output.txt", "Output file (default: output.txt)")
	flag.Parse()

	Info("##################################\n")
	Info("#                                #\n")
	Info("#          SessionProbe          #\n")
	Info("#                                #\n")
	Info("##################################\n\n")

	// check if the AppVersion was already set during compilation - otherwise manually get it from `./VERSION`
	CheckAppVersion()
	color.Yellow("Current version: %s\n\n", AppVersion)

	// the `cookie` and `urls` flags are required
 	if *cookiePtr == "" {
		ErrorLogger("Please provide a cookie using the -cookie argument\n")
		return
	}
	if *urlsPtr == "" {
		ErrorLogger("Please provide a urls file using the -urls argument\n")
		return
	}

	file, err := os.Open(*urlsPtr)
	if err != nil {
		ErrorLogger("%s\n", err)
		return
	}
	defer file.Close()

	outFile, err := os.Create(*outPtr)
	if err != nil {
		ErrorLogger("%s\n", err)
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
		ErrorLogger("%s\n", scanner.Err())
	}

	// total number of URLs
	urlCount := len(urls) 

	InfoLogger("Starting to check %d URLs with %d threads\n", urlCount, *threadPtr)

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
			statusCode := checkURL(url, *cookiePtr)
			// add URL to status code map
			urlStatusesMutex.Lock()
			urlStatuses[statusCode] = append(urlStatuses[statusCode], url)
			urlStatusesMutex.Unlock()

			// removing a slot in the semaphore so that a new goroutine can be created
			<-sem

			// increment the global counter
			atomic.AddInt32(&counter, 1)
			// print progress
			InfoLogger("Progress: %.2f%%\n", float64(counter)/float64(urlCount)*100)
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
			ErrorLogger("%s\n", err)
		}
		for _, url := range urlStatuses[statusCode] {
			_, err = outFile.WriteString(fmt.Sprintf("%s\n", url))
			if err != nil {
				ErrorLogger("%s\n", err)
			}
		}
		_, err = outFile.WriteString("\n")
		if err != nil {
			ErrorLogger("%s\n", err)
		}
	}
}

// function to do the HTTP request and check the response's status code
func checkURL(url, cookie string) int {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		ErrorLogger("%s\n", err)
		return 0
	}

	req.Header.Add("Cookie", cookie)
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// this will prevent redirect
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		ErrorLogger("%s\n", err)
		return 0
	}
	defer resp.Body.Close()

	return resp.StatusCode
}
