![Go Version](https://img.shields.io/github/go-mod/go-version/fw10/sessionprobe)

# SessionProbe ⚡
`SessionProbe` is a multi-threaded pentesting tool designed to assist in evaluating user privileges in web applications. It takes a user's session cookie and checks for a list of URLs if access is possible, highlighting potential authorization issues. `SessionProbe` deduplicates URL lists and provides real-time logging and progress tracking.

`SessionProbe` is intended to be used with `Burp Suite's` "Copy URLs in this host" functionality in the `Target` tab. 

# Built-in Help 🆘

Help is built-in!

- `sessionprobe --help` - outputs the help.

# How to Use ⚙

```text
Usage:
    sessionprobe [flags]

Flags:
  -cookie  string   Session cookie to be used in the requests (required)
  -urls    string   File containing the URLs to be checked (required)
  -out     string   Output file (default: ./output.txt)
  -threads int      Number of threads (default: 10)

Examples:
    ./sessionprobe -urls ./urls.txt -threads 15 -cookie ".AspNetCore.Cookies=<cookie>" -out ./output.txt
    ./sessionprobe -urls ./urls.txt -cookie ".AspNetCore.Cookies=<cookie>"
```

# Setup ✅

- You can simply run this tool from source via `go run .` 
- You can build the tool yourself via `go build`
