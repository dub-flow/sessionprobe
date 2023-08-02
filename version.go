package main

import (
	"fmt"
	"os"
)

var AppVersion string = "0.0.0"

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