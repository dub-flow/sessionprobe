package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/fatih/color"
	"github.com/hashicorp/go-version"
)


var AppVersion string = "0.0.0"
var latestRelease string = "https://github.com/dub-flow/sessionprobe/releases/latest"

func NotifyOfUpdates() {
	client := &http.Client{}
	req, err := http.NewRequest("GET", latestRelease, nil)
	if err != nil {
		return
	}

	req.Header.Add("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return
	}

	if resp.StatusCode != http.StatusOK {
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	var response map[string]interface{}

	err = json.Unmarshal(body, &response)
	if err != nil {
		return
	}

	vCurrent, err := version.NewVersion(AppVersion)
	if err != nil {
		fmt.Print(err)
	}

	vLatest, err := version.NewVersion(response["tag_name"].(string))
	if err != nil {
		fmt.Print(err)
	}

	// check if a newer version exists in the GitHub Releases
	if vCurrent.LessThan(vLatest) {
		color.Red(fmt.Sprintf("Please upgrade to the latest version of this tool (%s) by visiting %s\n\n", response["tag_name"], latestRelease))
	}
}

func CheckAppVersion() {
	if AppVersion == "0.0.0" {
		version, err := os.ReadFile("VERSION")
		if err != nil {
			fmt.Println(err)
		}

		// manually assign the value from `./VERSION` if it wasn't assigned during compilation already. This makes sure
		// that also people that run/build the app manually (without compiling the `./VERSION` into the binary) get the
		// appropriate version
		AppVersion = string(version)
	}
	// if the AppVersion is not "0.0.0" at this point, it means it has been set when compiling the app, so we just leave that
}