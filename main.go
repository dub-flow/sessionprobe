package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"io"
	"log"
	"net/http"
	"net"
	neturl "net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
    headers string
    urls    string
    threads int
    out     string
    proxy   string
	skipVerification bool
	filterRegex string
	ignoreCSS bool
	ignoreJS  bool
    green   = color.New(color.FgGreen).SprintFunc()
    red     = color.New(color.FgRed).SprintFunc()
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "sessionprobe",
		Short: "A tool for probing sessions",
		Long:  `SessionProbe is a tool for probing multiple sessions and recording their responses.`,
		Example: `./sessionprobe -u ./urls.txt
./sessionprobe -u ./urls.txt --out ./unauthenticated-test.txt
./sessionprobe -u ./urls.txt --threads 15 -H "Cookie: .AspNetCore.Cookies=<cookie>" -o ./output.txt
./sessionprobe -u ./urls.txt -H "Authorization: Bearer <token>" --proxy http://localhost:8080
./sessionprobe -u ./urls.txt -r "Page Not Found"`,
		Run:   run,
	}

	rootCmd.PersistentFlags().StringVarP(&headers, "headers", "H", "", "HTTP headers to be used in the requests in the format \"Key1:Value1;Key2:Value2;...\"")
	rootCmd.PersistentFlags().StringVarP(&urls, "urls", "u", "", "file containing the URLs to be checked (required)")
	rootCmd.PersistentFlags().IntVarP(&threads, "threads", "t", 10, "number of threads")
	rootCmd.PersistentFlags().StringVarP(&out, "out", "o", "output.txt", "output file")
	rootCmd.PersistentFlags().StringVarP(&proxy, "proxy", "p", "", "proxy URL (default: \"\")")
	rootCmd.PersistentFlags().BoolVar(&skipVerification, "skip-verification", false, "skip verification of SSL certificates (default false)")
	rootCmd.PersistentFlags().BoolVar(&ignoreCSS, "ignore-css", true, "ignore URLs ending with .css")
	rootCmd.PersistentFlags().BoolVar(&ignoreJS, "ignore-js", true, "ignore URLs ending with .js")
	rootCmd.PersistentFlags().StringVarP(&filterRegex, "filter-regex", "r", "", "Filter HTTP responses using this regex. Responses matching this regex will not be part of the output.")

	rootCmd.Execute()
}

// run() gets executed when the root command is called
func run(cmd *cobra.Command, args []string) {
	printIntro()

	// check if the AppVersion was already set during compilation - otherwise manually get it from `./current_version`
	CheckAppVersion()
	color.Yellow("Current version: %s\n\n", AppVersion)

	// the `urls` flag is required
	if urls == "" {
		Error("Please provide a URLs file using the '-urls <path_to_urls_file>' argument.")
		Error("Use --help for more information.")
		return
	}

	if ignoreCSS {
		Info("Ignoring URLs that end with .css")
	}

	if ignoreJS {
		Info("Ignoring URLs that end with .js")
	}

	var headersMap map[string]string
	if headers != "" {
		headersMap = parseHeaders(headers)
	}

	// if a proxy was provided, check if the proxy is reachable. Exit if it's not
	if proxy != "" {
		checkProxyReachability(proxy)
	}

	// compile the regex provided via `-fr`
	var compiledRegex *regexp.Regexp
	if filterRegex != "" {
		var err error
		compiledRegex, err = regexp.Compile(filterRegex)
		if err != nil {
			Error("Invalid regex: %s", err)
			return
		}
	}

	file, err := os.Open(urls)
	if err != nil {
		Error("%s", err)
		return
	}
	defer file.Close()

	// create semaphore with the specified number of threads
	sem := make(chan bool, threads)
	// make sure to wait for all threads to finish before exiting the program
	var wg sync.WaitGroup

	// using a map to deduplicate URLs
	urlsMap := readURLs(file)

	// total number of URLs
	urlCount := len(urlsMap)

	Info("Starting to check %d URLs (deduplicated) with %d threads", urlCount, threads)

	// map to store URLs by status code
	urlStatuses := processURLs(urlsMap, headersMap, proxy, &wg, sem, compiledRegex)

	// wait for all threads to finish
	wg.Wait()

	outFile, err := os.Create(out)
	if err != nil {
		Error("%s", err)
		return
	}
	defer outFile.Close()

	writeToFile(urlStatuses, outFile)
}

func printIntro() {
	color.Green("##################################\n")
	color.Green("#                                #\n")
	color.Green("#          SessionProbe          #\n")
	color.Green("#                                #\n")
	color.Green("##################################\n\n")
}

func readURLs(file *os.File) map[string]bool {
    // read the URLs line by line
    scanner := bufio.NewScanner(file)

    // deduplicate URLs
    urls := make(map[string]bool)
    for scanner.Scan() {
        url := scanner.Text()

        if (ignoreCSS && strings.HasSuffix(url, ".css")) || 
           (ignoreJS && strings.HasSuffix(url, ".js")) {
            continue
        }

        urls[url] = true
    }

    if scanner.Err() != nil {
        Error("%s", scanner.Err())
    }

    return urls
}

func processURLs(urls map[string]bool, headers map[string]string, proxy string, wg *sync.WaitGroup, sem chan bool, compiledRegex *regexp.Regexp) map[int][]string {
    // map to store URLs by status code
	urlStatuses := make(map[int][]string)
	var urlStatusesMutex sync.Mutex

	// for the progress counter
	var processedCount int32
	totalUrls := int32(len(urls))


	// process each URL in the deduplicated map
	for url := range urls {
		wg.Add(1)

		// will block if there is already `threads` threads running
		sem <- true

		// launch a new goroutine for each URL
		go func(url string) {
			// using defer to ensure the semaphore is released and the waitgroup is decremented regardless of where we exit in the function
			defer func() {
				// always release the semaphore token
				<-sem
				// always decrement the waitgroup counter
				wg.Done()

				// increment the processedCount and log progress
				atomic.AddInt32(&processedCount, 1)
				percentage := float64(processedCount) / float64(totalUrls) * 100
				Info("Progress: %.2f%% (%d/%d URLs processed)", percentage, processedCount, totalUrls)
			}()
		
			// Now use checkURL function instead of http.Get
			statusCode, _, matched := checkURL(url, headers, proxy, compiledRegex)
			if matched {
				urlStatusesMutex.Lock()
				urlStatuses[statusCode] = append(urlStatuses[statusCode], url)
				urlStatusesMutex.Unlock()
			}
		}(url)
	}

	return urlStatuses
}

// takes a map of HTTP status codes to URLs and writes it to the output file
func writeToFile(urlStatuses map[int][]string, outFile *os.File) {
    writer := bufio.NewWriter(outFile)

    // sort the map keys to ensure consistent output
    var keys []int
    for k := range urlStatuses {
        keys = append(keys, k)
    }
    sort.Ints(keys)

    for _, k := range keys {
        _, _ = writer.WriteString(fmt.Sprintf("HTTP Status: %d\n", k))
        for _, url := range urlStatuses[k] {
            _, _ = writer.WriteString(url + "\n")
        }
        _, _ = writer.WriteString("\n")
    }
    writer.Flush()
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
func checkURL(url string, headers map[string]string, proxy string, compiledRegex *regexp.Regexp) (int, int, bool) {
	client := createHTTPClient(proxy) 
    req, err := prepareHTTPRequest(url, headers)
    
    if err != nil {
        Error("Failed to create request: %s", err)
        return 0, 0, false
    }

    resp, err := client.Do(req)
    if handleHTTPError(err, url) {
        return 0, 0, false
    }
    defer resp.Body.Close()

    bodyBytes, err := readResponseBody(resp.Body, url)
    if err != nil {
        return resp.StatusCode, 0, false
    }

    // if a regex pattern is provided, check if the response matches
    return processResponse(resp.StatusCode, bodyBytes, compiledRegex)
}

// setting up the HTTP client with potential proxy and other configurations
// setting up the HTTP client with potential proxy and other configurations
func createHTTPClient(proxy string) *http.Client {
	proxyURLFunc := func(_ *http.Request) (*neturl.URL, error) {
		return neturl.Parse(proxy)
	}
	
	if proxy == "" {
		proxyURLFunc = http.ProxyFromEnvironment
	}
	
    return &http.Client{
        Transport: &http.Transport{
            Proxy: proxyURLFunc,
            TLSClientConfig: &tls.Config{
                // skip SSL verification if specified
                InsecureSkipVerify: skipVerification,
            },
        },
        Timeout: 10 * time.Second, // set timeout for HTTP requests
    }
}

// create a new HTTP request and set the provided headers
func prepareHTTPRequest(url string, headers map[string]string) (*http.Request, error) {
    req, err := http.NewRequest("GET", url, nil)
    if err != nil {
        return nil, err
    }

    // add custom headers to the request if any provided
    for key, value := range headers {
        req.Header.Set(key, value)
    }

    return req, nil
}

func handleHTTPError(err error, url string) bool {
    if err != nil {
        if _, ok := err.(net.Error); ok {
			// log network errors separately
            Error("Network error for URL: %s - %s", url, err) 
            return true
        }
		// log other errors
        Error("Error fetching URL: %s - %s", url, err) 
        return true
    }
    return false
}

func readResponseBody(body io.ReadCloser, url string) ([]byte, error) {
    bodyBytes, err := io.ReadAll(body)
    if err != nil {
        Error("Error reading response body for URL: %s - %s", url, err) 
        return nil, err
    }
    return bodyBytes, nil
}

func processResponse(statusCode int, bodyBytes []byte, compiledRegex *regexp.Regexp) (int, int, bool) {
    if compiledRegex != nil && compiledRegex.Match(bodyBytes) {
        return statusCode, len(bodyBytes), true
    }
    return statusCode, len(bodyBytes), false
}

func checkProxyReachability(proxy string) {
	if proxy != "" {
		proxyURL, err := neturl.Parse(proxy)
		if err != nil {
			Error("Failed to parse proxy URL: %s", err)
			os.Exit(1)
		}

		_, err = net.DialTimeout("tcp", proxyURL.Host, 5*time.Second)
		if err != nil {
			Error("Failed to connect to the proxy: %s", err)
			os.Exit(1)
		}
	}
}

func Info(format string, a ...interface{}) {
	log.Printf("%s", green(fmt.Sprintf(format, a...)))
}

func Error(format string, a ...interface{}) {
	log.Printf("%s", red(fmt.Sprintf(format, a...)))
}
