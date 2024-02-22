package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	neturl "net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	headers          string
	urls             string
	threads          int
	out              string
	proxy            string
	skipVerification bool
	filterRegex      string
	filterLengths    string
	ignoreCSS        bool
	ignoreJS         bool
	methodPOST       bool
	methodPUT        bool
	methodDELETE     bool
	methodPATCH      bool
	methodOPTIONS    bool
	methodALL        bool
	green            = color.New(color.FgGreen).SprintFunc()
	red              = color.New(color.FgRed).SprintFunc()
	yellow           = color.New(color.FgYellow).SprintFunc()
)

type Result struct {
	Method string
	URL    string
	Length int
}

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
		Run: run,
	}

	rootCmd.PersistentFlags().StringVarP(&headers, "headers", "H", "", "HTTP headers to be used in the requests in the format \"Key1:Value1;Key2:Value2;...\"")
	rootCmd.PersistentFlags().StringVarP(&urls, "urls", "u", "", "file containing the URLs to be checked (required)")
	rootCmd.PersistentFlags().IntVarP(&threads, "threads", "t", 10, "number of threads")
	rootCmd.PersistentFlags().StringVarP(&out, "out", "o", "output.txt", "output file")
	rootCmd.PersistentFlags().StringVarP(&proxy, "proxy", "p", "", "proxy URL (default: \"\")")
	rootCmd.PersistentFlags().BoolVar(&skipVerification, "skip-verification", false, "skip verification of SSL certificates (default false)")
	rootCmd.PersistentFlags().BoolVar(&ignoreCSS, "ignore-css", true, "ignore URLs ending with .css")
	rootCmd.PersistentFlags().BoolVar(&ignoreJS, "ignore-js", true, "ignore URLs ending with .js")
	rootCmd.PersistentFlags().StringVarP(&filterRegex, "filter-regex", "r", "", "Exclude HTTP responses using a regex. Responses whose body matches this regex will not be part of the output.")
	rootCmd.PersistentFlags().StringVarP(&filterLengths, "filter-lengths", "l", "", "Exclude HTTP responses by body length. You can specify lengths separated by commas (e.g., \"123,456,789\").")
	rootCmd.PersistentFlags().BoolVar(&methodPOST, "check-post", false, "Check POST method (default false)")
	rootCmd.PersistentFlags().BoolVar(&methodPUT, "check-put", false, "Check PUT method (default false)")
	rootCmd.PersistentFlags().BoolVar(&methodDELETE, "check-delete", false, "Check DELETE method (default false)")
	rootCmd.PersistentFlags().BoolVar(&methodPATCH, "check-patch", false, "Check PATCH method (default false)")
	rootCmd.PersistentFlags().BoolVar(&methodOPTIONS, "check-options", false, "Check OPTIONS method (default false)")
	rootCmd.PersistentFlags().BoolVar(&methodALL, "check-all", false, "Check POST, DELETE, PUT, PATCH & OPTIONS methods (default false)")

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

	var headersMap map[string][]string
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

	// map to store URLs by status code
	excludedLengths := parseLengths(filterLengths)
	urlStatuses := processURLs(urlsMap, headersMap, proxy, &wg, sem, compiledRegex, excludedLengths)

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

func getMethods() []string {
	out := []string{"GET"}
	Info("Running GET requests against every URL")

	if methodALL || methodPOST {
		out = append(out, "POST")
		Warn("Also running POST requests against every URL (this feature is currently in its initial development phase)")
		Warn("It currently sends a request to each URL with an empty body and observes the response")
	}

	if methodALL || methodPUT {
		out = append(out, "PUT")
		Warn("Also running PUT requests against every URL (this feature is currently in its initial development phase)")
		Warn("It currently sends a request to each URL with an empty body and observes the response")
	}

	if methodALL || methodPATCH {
		out = append(out, "PATCH")
		Warn("Also running PATCH requests against every URL (this feature is currently in its initial development phase)")
		Warn("It currently sends a request to each URL with an empty body and observes the response")
	}

	if methodALL || methodDELETE {
		out = append(out, "DELETE")
		Info("Also running DELETE requests against every URL")
	}

	if methodALL || methodOPTIONS {
		out = append(out, "OPTIONS")
		Info("Also running OPTIONS requests against every URL")
	}

	return out
}

func processURLs(urls map[string]bool, headers map[string][]string, proxy string, wg *sync.WaitGroup, sem chan bool, compiledRegex *regexp.Regexp, allowedLengths map[int]bool) map[int][]Result {
	// map to store URLs by status code
	urlStatuses := make(map[int][]Result)
	var urlStatusesMutex sync.Mutex

	var methods []string = getMethods()

	// for the progress counter
	var processedCount int32
	totalUrls := int32(len(urls))
	totalMethods := int32(len(methods))
	totalRequests := totalUrls * totalMethods

	Info("Starting to check %d unique URLs (deduplicated) and %d methods => %d requests", totalUrls, totalMethods, totalRequests)
	Info("We use %d threads", threads)

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
			}()

			// inside the goroutine of processURLs
			for _, method := range methods {
				statusCode, length, matched := checkURL(method, url, headers, proxy, compiledRegex, allowedLengths)
				if matched {
					urlStatusesMutex.Lock()
					urlStatuses[statusCode] = append(urlStatuses[statusCode], Result{Method: method, URL: url, Length: length})
					urlStatusesMutex.Unlock()
				}

				// increment the processedCount and log progress
				atomic.AddInt32(&processedCount, 1)
				percentage := float64(processedCount) / float64(totalRequests) * 100
				Info("Progress: %.2f%% (%d/%d deduped URLs processed)", percentage, processedCount, totalRequests)
			}

		}(url)
	}

	return urlStatuses
}

// takes a map of HTTP status codes to URLs and writes it to the output file
func writeToFile(urlStatuses map[int][]Result, outFile *os.File) {
	writer := bufio.NewWriter(outFile)

	// sort the map keys to ensure consistent output
	var keys []int
	for k := range urlStatuses {
		keys = append(keys, k)
	}
	sort.Ints(keys)

	for _, k := range keys {
		_, _ = writer.WriteString(fmt.Sprintf("Responses with Status Code: %d\n\n", k))
		for _, result := range urlStatuses[k] {
			_, _ = writer.WriteString(fmt.Sprintf("| %s | %s => Length: %d\n", result.Method, result.URL, result.Length))
		}
		_, _ = writer.WriteString("\n")
	}

	writer.Flush()
}

func parseLengths(lengths string) map[int]bool {
	lengthsMap := make(map[int]bool)

	// if the input string is empty (i.e. `-l` was not provided), return an empty map
	if lengths == "" {
		return lengthsMap
	}

	parts := strings.Split(lengths, ",")
	for _, part := range parts {
		length, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			Error("Invalid length: %s", part)
			continue
		}
		lengthsMap[length] = true
	}

	return lengthsMap
}

func parseHeaders(headers string) map[string][]string {
    headerMap := make(map[string][]string)
    pairs := strings.Split(headers, ";")

    for _, pair := range pairs {
        parts := strings.SplitN(pair, ":", 2)

        if len(parts) != 2 {
            Error("Invalid header format: %s", pair)
            continue
        }

        key := strings.TrimSpace(parts[0])
        value := strings.TrimSpace(parts[1])

        // accumulate headers of the same key, required for e.g. setting multiple cookies
        headerMap[key] = append(headerMap[key], value)
    }

    return headerMap
}

// function to do the HTTP request and check the response's status code and response length
func checkURL(method string, url string, headers map[string][]string, proxy string, compiledRegex *regexp.Regexp, allowedLengths map[int]bool) (int, int, bool) {
	client := createHTTPClient(proxy)
	req, err := prepareHTTPRequest(method, url, headers)

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
	return filterResponseByLengthAndRegex(resp.StatusCode, bodyBytes, compiledRegex, allowedLengths)
}

// setting up the HTTP client with potential proxy and other configurations
func createHTTPClient(proxy string) *http.Client {
	proxyURLFunc := func(_ *http.Request) (*neturl.URL, error) {
		return neturl.Parse(proxy)
	}

	if proxy == "" {
		proxyURLFunc = http.ProxyFromEnvironment
	}

	// custom CheckRedirect function that always returns an error. This prevents the client from following any redirects
	noRedirect := func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	return &http.Client{
		Transport: &http.Transport{
			Proxy: proxyURLFunc,
			TLSClientConfig: &tls.Config{
				// skip SSL verification if specified
				InsecureSkipVerify: skipVerification,
			},
		},
		Timeout:       10 * time.Second, // set timeout for HTTP requests
		CheckRedirect: noRedirect,       // Set the custom redirect policy
	}
}

// create a new HTTP request and set the provided headers
func prepareHTTPRequest(method string, url string, headers map[string][]string) (*http.Request, error) {
    req, err := http.NewRequest(method, url, nil)
    if err != nil {
        return nil, err
    }

    // Add custom headers to the request. If multiple cookies are provided, concatenate them.
    for key, values := range headers {
        if key == "Cookie" {
            // Join multiple cookie values into a single header
            req.Header.Set(key, strings.Join(values, "; "))
        } else {
            // For other headers, just set the first value (modify this as needed)
            for _, value := range values {
                req.Header.Add(key, value)
            }
        }
    }

    return req, nil
}

func handleHTTPError(err error, url string) bool {
	if err != nil {
		if _, ok := err.(net.Error); ok {
			// log network errors separately
			Error("Network error for URL: %s - %s", url, err)

			// provide a hint for the 'x509: certificate signed by unknown authority' error
			if strings.Contains(err.Error(), "x509") {
				Error("You may be able to fix the x509 certificate error by providing the --skip-verification flag")
			}

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

func filterResponseByLengthAndRegex(statusCode int, bodyBytes []byte, compiledRegex *regexp.Regexp, excludedLengths map[int]bool) (int, int, bool) {
	length := len(bodyBytes)

	// If the length is in the excludedLengths map, exclude the response.
	if excludedLengths[length] {
		return statusCode, length, false
	}

	// If there's no regex provided, don't filter out any responses.
	if compiledRegex == nil {
		return statusCode, length, true
	}

	// If a regex is provided, only return true if the response does NOT match the regex
	if !compiledRegex.Match(bodyBytes) {
		return statusCode, length, true
	}

	return statusCode, length, false
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
			Error("In case you're using docker to run the app, remember that you can't refer to the proxy as 'localhost' but need its IP :)")
			os.Exit(1)
		}
	}
}

func Info(format string, a ...interface{}) {
	log.Printf("%s", green(fmt.Sprintf(format, a...)))
}

func Warn(format string, a ...interface{}) {
	log.Printf("%s", yellow(fmt.Sprintf(format, a...)))
}

func Error(format string, a ...interface{}) {
	log.Printf("%s", red(fmt.Sprintf(format, a...)))
}
