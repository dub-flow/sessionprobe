package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
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

func TestProcessResponse(t *testing.T) {
	compiledRegex, _ := regexp.Compile("Hello")
	statusCode := 200
	body := []byte("Hello, World!")
	expectedStatus, expectedLength, expectedMatched := 200, len(body), true

	actualStatus, actualLength, actualMatched := processResponse(statusCode, body, compiledRegex)

	if actualStatus != expectedStatus || actualLength != expectedLength || actualMatched != expectedMatched {
		t.Errorf("Mismatch in processResponse output")
	}
}

func TestCheckURL(t *testing.T) {
	// Mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello, World!"))
	}))
	defer server.Close()

	headers := make(map[string]string)
	proxy := ""
	compiledRegex, _ := regexp.Compile("Hello")
	expectedStatus, expectedMatched := 200, true

	actualStatus, _, actualMatched := checkURL(server.URL, headers, proxy, compiledRegex)

	if actualStatus != expectedStatus || actualMatched != expectedMatched {
		t.Errorf("Mismatch in checkURL output")
	}
}

func TestSessionProbeIntegration(t *testing.T) {
	// 1. Set up a mock HTTP server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/200":
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, "OK")
		case "/404":
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintln(w, "Not Found")
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer ts.Close()

	// Create a temporary URLs file for the test
	urlsFile, err := os.CreateTemp("", "test-urls-*.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(urlsFile.Name())

	urls := []string{
		ts.URL + "/200",
		ts.URL + "/404",
		ts.URL + "/500",
	}
	for _, url := range urls {
		if _, err := urlsFile.WriteString(url + "\n"); err != nil {
			t.Fatalf("Failed to write to temp file: %v", err)
		}
	}
	urlsFile.Close()

	// 2. Use go run main.go to probe the mock server
	outputFile := "test-output.txt"
	defer os.Remove(outputFile)

	cmd := exec.Command("go", "run", ".", "-u", urlsFile.Name(), "-o", outputFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to run main.go: %v", err)
	}
	// 3. Check the output
	output, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}
	if !strings.Contains(string(output), "Status Code: 200") ||
		!strings.Contains(string(output), "Status Code: 404") ||
		!strings.Contains(string(output), "Status Code: 500") {
		t.Fatalf("Unexpected output: %v", string(output))
	}
}