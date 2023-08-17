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
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var counter int32
var (
    headers string
    urls    string
    threads int
    out     string
    proxy   string
	skipVerification bool
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
		Example: `./sessionprobe -u ./urls.txt --out ./unauthenticated-test.txt
./sessionprobe -u ./urls.txt --threads 15 -H "Cookie: .AspNetCore.Cookies=<cookie>" -o ./output.txt
./sessionprobe -u ./urls.txt -H "Authorization: Bearer <token>" --proxy http://localhost:8080`,
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
	urlStatuses := processURLs(urlsMap, headersMap, &proxy, &wg, sem)

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


func processURLs(urls map[string]bool, headers map[string]string, proxyPtr *string, wg *sync.WaitGroup, sem chan bool) map[int][]string {
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
			Info("Progress: %.2f%%", float64(counter)/float64(len(urls))*100)
		}(url)
	}

	return urlStatuses
}

func writeToFile(urlStatuses map[int][]string, outFile *os.File) {
	// get the list of status codes
	statusCodes := make([]int, 0, len(urlStatuses))
	for statusCode := range urlStatuses {
		statusCodes = append(statusCodes, statusCode)
	}
	// sort the status codes
	sort.Ints(statusCodes)

	// write output to file, sorted by status code
	for _, statusCode := range statusCodes {
		_, err := outFile.WriteString(fmt.Sprintf("Responses with Status Code: %d\n\n", statusCode))
		if err != nil {
			Error("%s", err)
		}

		// sort URLs by their path for each status code
		sort.Slice(urlStatuses[statusCode], func(i, j int) bool {
			// extracting the path from the URL string, which follows the format "URL => Length: x"
			urlI, errI := neturl.Parse(strings.Split(urlStatuses[statusCode][i], " ")[0])
			urlJ, errJ := neturl.Parse(strings.Split(urlStatuses[statusCode][j], " ")[0])

			// if there's an error parsing the URL, return false so the order doesn't change
			if errI != nil || errJ != nil {
				return false
			}

			// Compare the path of the URLs
			return urlI.Path < urlJ.Path
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

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: skipVerification},
	}

	if proxy != "" {
		// if a proxy was provided via `-proxy`.. Note that we already verified that the proxy is reachable at this point
		proxyURL, _ := neturl.Parse(proxy)
		tr.Proxy = http.ProxyURL(proxyURL)
	}

	client = &http.Client{
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		if strings.Contains(err.Error(), "x509: certificate signed by unknown authority") && !skipVerification {
			Error("\"x509: certificate signed by unknown authority\" occurred. Please install the certificate of the proxy at OS-level or provide the --skip-verification flag")
			os.Exit(1)
		}
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
