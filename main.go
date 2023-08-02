package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"sync"
	"sync/atomic"
)

var counter int32 // global counter for URLs processed

func main() {
	cookiePtr := flag.String("cookie", "", "Session cookie to be used in the requests")
	urlsPtr := flag.String("urls", "", "File containing the URLs to be checked")
	threadPtr := flag.Int("threads", 10, "Number of threads (default: 10)")
	outPtr := flag.String("out", "output.txt", "Output file (default: output.txt)")
	flag.Parse()

	log.Println("##############################")
	log.Println("#                            #")
	log.Println("#        SessionProbe        #")
	log.Println("#                            #")
	log.Println("###############################" + "\n\n")

	// the `cookie` and `urls` flags are required
 	if *cookiePtr == "" {
		log.Fatalln("Please provide a cookie using the -cookie argument")
		return
	}
	if *urlsPtr == "" {
		log.Fatalln("Please provide a urls file using the -urls argument")
		return
	}

	file, err := os.Open(*urlsPtr)
	if err != nil {
		log.Fatalln(err)
		return
	}
	defer file.Close()

	outFile, err := os.Create(*outPtr)
	if err != nil {
		log.Fatalln(err)
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
		log.Fatalln(scanner.Err())
	}

	urlCount := len(urls) // total number of URLs

	log.Printf("Starting to check %d URLs with %d threads\n", urlCount, *threadPtr)

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
			log.Printf("Progress: %.2f%%\n", float64(counter)/float64(urlCount)*100)
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
			log.Println(err)
		}
		for _, url := range urlStatuses[statusCode] {
			_, err = outFile.WriteString(fmt.Sprintf("%s\n", url))
			if err != nil {
				log.Println(err)
			}
		}
		_, err = outFile.WriteString("\n")
		if err != nil {
			log.Println(err)
		}
	}
}

// function to do the HTTP request and check the response's status code
func checkURL(url, cookie string) int {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Println(err)
		return 0
	}

	req.Header.Add("Cookie", cookie)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println(err)
		return 0
	}
	defer resp.Body.Close()

	return resp.StatusCode
}
