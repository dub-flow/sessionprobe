package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestParseHeaders(t *testing.T) {
	headersString := "Key1:Value1;Key2:Value2"
	expected := map[string]string{
		"Key1": "Value1",
		"Key2": "Value2",
	}

	result := parseHeaders(headersString)
	for k, v := range expected {
		if resultV, ok := result[k]; !ok || v != resultV {
			t.Errorf("Expected %s for key %s but got %s", v, k, resultV)
		}
	}
}

func TestProcessResponse_MatchesRegex(t *testing.T) {
	compiledRegex, _ := regexp.Compile("World")
	statusCode := 200
	body := []byte("Hello, World!")
	expectedStatus, expectedLength, expectedMatched := 200, len(body), false
	excludedLengths := make(map[int]bool)

	actualStatus, actualLength, actualMatched := filterResponseByLengthAndRegex(statusCode, body, compiledRegex, excludedLengths)

	if actualStatus != expectedStatus || actualLength != expectedLength || actualMatched != expectedMatched {
		t.Errorf("Expected status %d, length %d, matched %v but got status %d, length %d, matched %v",
			expectedStatus, expectedLength, expectedMatched, actualStatus, actualLength, actualMatched)
	}
}

func TestProcessResponse_DoesNotMatchRegex(t *testing.T) {
	compiledRegex, _ := regexp.Compile("Bye")
	statusCode := 200
	body := []byte("Hello, World!")
	expectedStatus, expectedLength, expectedMatched := 200, len(body), true
	excludedLengths := make(map[int]bool)

	actualStatus, actualLength, actualMatched := filterResponseByLengthAndRegex(statusCode, body, compiledRegex, excludedLengths)

	if actualStatus != expectedStatus || actualLength != expectedLength || actualMatched != expectedMatched {
		t.Errorf("Expected status %d, length %d, matched %v but got status %d, length %d, matched %v",
			expectedStatus, expectedLength, expectedMatched, actualStatus, actualLength, actualMatched)
	}
}

func TestCheckURL_MatchesRegex(t *testing.T) {
	// Mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello, World!"))
	}))
	defer server.Close()

	headers := make(map[string]string)
	proxy := ""
	compiledRegex, _ := regexp.Compile("World")   // Matching regex
	expectedStatus, expectedMatched := 200, false // It should filter out the response because it matches
	excludedLengths := make(map[int]bool)

	actualStatus, _, actualMatched := checkURL("GET", server.URL, headers, proxy, compiledRegex, excludedLengths)

	if actualStatus != expectedStatus || actualMatched != expectedMatched {
		t.Errorf("Expected status %d, matched %v but got status %d, matched %v",
			expectedStatus, expectedMatched, actualStatus, actualMatched)
	}
}

func TestCheckURL_DoesNotMatchRegex(t *testing.T) {
	// Mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello, World!"))
	}))
	defer server.Close()

	headers := make(map[string]string)
	proxy := ""
	compiledRegex, _ := regexp.Compile("Bye")    // Non-matching regex
	expectedStatus, expectedMatched := 200, true // It should not filter out the response because it doesn't match
	excludedLengths := make(map[int]bool)

	actualStatus, _, actualMatched := checkURL("GET", server.URL, headers, proxy, compiledRegex, excludedLengths)

	if actualStatus != expectedStatus || actualMatched != expectedMatched {
		t.Errorf("Expected status %d, matched %v but got status %d, matched %v",
			expectedStatus, expectedMatched, actualStatus, actualMatched)
	}
}

func TestCheckURL_ExcludedLength(t *testing.T) {
	// Mock HTTP server with a fixed response length
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello, World!")) // Length is 13
	}))
	defer server.Close()

	headers := make(map[string]string)
	proxy := ""
	compiledRegex, _ := regexp.Compile(".*")      // Matching any string
	expectedStatus, expectedMatched := 200, false // It should filter out the response because of its length
	excludedLengths := map[int]bool{
		13: true, // Excluding the length 13
	}

	actualStatus, _, actualMatched := checkURL("GET", server.URL, headers, proxy, compiledRegex, excludedLengths)

	if actualStatus != expectedStatus || actualMatched != expectedMatched {
		t.Errorf("Expected status %d, matched %v but got status %d, matched %v",
			expectedStatus, expectedMatched, actualStatus, actualMatched)
	}
}

func TestFilterRegexFunctionality(t *testing.T) {
	// Ensure the 'testing' directory exists.
	EnsureOutputFolderExists(t)

	// 1. Set up a mock HTTP server.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/shouldInclude":
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, "This should be included.")
		case "/shouldExclude":
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, "This should not be included.")
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer ts.Close()

	// Use filepath.Join to construct the file path in an OS-agnostic manner.
	urlsFilePath := filepath.Join(".", "testing", "test-urls-regex.txt")
	// Open (or create if it doesn't exist) and truncate the URLs file.
	urlsFile, err := os.OpenFile(urlsFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatalf("Failed to open or create test file: %v", err)
	}
	defer urlsFile.Close()

	urls := []string{
		ts.URL + "/shouldInclude",
		ts.URL + "/shouldExclude",
	}
	for _, url := range urls {
		if _, err := urlsFile.WriteString(url + "\n"); err != nil {
			t.Fatalf("Failed to write to test file: %v", err)
		}
	}

	// 2. Use go run main.go with filter-regex to probe the mock server.
	outputFile := filepath.Join(".", "testing", "test-output-regex.txt")
	cmd := exec.Command("go", "run", ".", "-u", urlsFilePath, "-o", outputFile, "--filter-regex", "This should not be included.")
	// cmd.Stdout = os.Stdout
	// cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to run main.go: %v", err)
	}

	// 3. Check the output.
	output, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}
	outputStr := string(output)

	// Check if the URL of the "excluded" endpoint is present, even though its body should have caused it to be filtered out.
	if strings.Contains(outputStr, ts.URL+"/shouldExclude") {
		t.Fatalf("URL that should have been filtered by regex was found in the output: %v", outputStr)
	}

	// Check if the URL of the "included" endpoint is present.
	if !strings.Contains(outputStr, ts.URL+"/shouldInclude") {
		t.Fatalf("URL that shouldn't be filtered by regex was missing from the output: %v", outputStr)
	}
}

// ensures the ./testing folder exists for test files
func EnsureOutputFolderExists(t *testing.T) {
	dir := filepath.Join(".", "testing")

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.Mkdir(dir, 0755); err != nil {
			t.Fatalf("Failed to create ./testing directory: %v", err)
		}
	}
}
