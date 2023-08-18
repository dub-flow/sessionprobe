![Go Version](https://img.shields.io/github/go-mod/go-version/fw10/sessionprobe)
![GitHub Downloads](https://img.shields.io/github/downloads/fw10/sessionprobe/total)
![Docker Image Size](https://img.shields.io/docker/image-size/fw10/sessionprobe/latest)
![Docker Pulls](https://img.shields.io/docker/pulls/fw10/sessionprobe)

# SessionProbe 🚀⚡

`SessionProbe` is a multi-threaded pentesting tool designed to assist in evaluating user privileges in web applications. It takes a user's session token and checks for a list of URLs if access is possible, highlighting potential authorization issues. `SessionProbe` deduplicates URL lists and provides real-time logging and progress tracking.

`SessionProbe` is intended to be used with `Burp Suite's` "Copy URLs in this host" functionality in the `Target` tab (available in the free `Community Edition`). 

**Note**: You may want to change the `filter` in `Burps's` `Target` tab to include files or images. Otherwise, these `URLs` would not be copied by "Copy URLs in this host" and would not be tested by `SessionProbe`.

# Built-in Help 🆘

Help is built-in!

- `sessionprobe --help` - outputs the help.

# How to Use ⚙

```text
Usage:
    sessionprobe [flags]

Flags:
  -u, --urls string         file containing the URLs to be checked (required)
  -H, --headers string      HTTP headers to be used in the requests in the format "Key1:Value1;Key2:Value2;..."
  -h, --help                help for sessionprobe
      --ignore-css          ignore URLs ending with .css (default true)
      --ignore-js           ignore URLs ending with .js (default true)
  -o, --out string          output file (default "output.txt")
  -p, --proxy string        proxy URL (default: "")
      --skip-verification   skip verification of SSL certificates (default false)
  -t, --threads int         number of threads (default 10)

Examples:
    ./sessionprobe -u ./urls.txt
    ./sessionprobe -u ./urls.txt --out ./unauthenticated-test.txt --threads 15
    ./sessionprobe -u ./urls.txt -H "Cookie: .AspNetCore.Cookies=<cookie>" -o ./output.txt
    ./sessionprobe -u ./urls.txt -H "Authorization: Bearer <token>" --proxy http://localhost:8080
```

# Run via Docker 🐳

1. Navigate into the directory where your `URLs file` is.
2. Run the below command:
```text
docker run -it --rm -v "$(pwd):/app/files" --name sessionprobe fw10/sessionprobe [flags]
```
  - Note that we are mounting the current directory in. This means that your `URLs file` must be in the current directory and your `output file` will also be in this directory.
  - Also remember to have a `Burp listener` run on all interfaces if you want to use the `--proxy` option

# Setup ✅

- You can simply run this tool from source via `go run .` 
- You can build the tool yourself via `go build`
- You can build the docker image yourself via `docker build . -t fw10/sessionprobe`

# Features 🔎 

- Test for authorization issues
- Automatically dedupes URLs
- Sorts the URLs by response status code and extension (e.g., `.css`, `.js`), and provides the length
- Multi-threaded
- Proxy functionality to pass all requests e.g. through `Burp`
- ...

# Example Output 📋

```
Responses with Status Code: 200

https://example.com/<some-path> => Length: 12345
https://example.com/<some-path> => Length: 40
...

Responses with Status Code: 301

https://example.com/<some-path> => Length: 890
https://example.com/<some-path> => Length: 434
...

Responses with Status Code: 302

https://example.com/<some-path> => Length: 0
...

Responses with Status Code: 404

...

Responses with Status Code: 502

...

```

# Releases 🔑 

- The `Releases` section contains some already compiled binaries for you so that you might not have to build the tool yourself
- For the `Mac releases`, your Mac may throw a warning (`"cannot be opened because it is from an unidentified developer"`)
    - To avoid this warning in the first place, you could simply build the app yourself (see `Setup`)
    - Alternatively, you may - at your own risk - bypass this warning following the guidance here: https://support.apple.com/guide/mac-help/apple-cant-check-app-for-malicious-software-mchleab3a043/mac
    - Afterwards, you can simply run the binary from the command line and provide the required flags

# Bug Reports 🐞

If you find a bug, please file an Issue right here in GitHub, and I will try to resolve it in a timely manner.
