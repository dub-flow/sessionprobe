package main

import (
	"net/http"
	"net/http/httptest"
	"regexp"
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

