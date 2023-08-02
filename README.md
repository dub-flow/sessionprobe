# SessionProbe ⚡
`SessionProbe` is a multi-threaded pentesting tool designed to assist in evaluating user privileges in web applications. It takes a user's session cookie and checks for a list of URLs if access is possible, highlighting potential authorization issues. `SessionProbe` deduplicates URL lists and provides real-time logging and progress tracking.

`SessionProbe` is intended to be used with `Burp Suite's` "Copy URLs in this host" functionality in the `Target` tab. 

<screenshot of the feature>

# Built-in Help 🆘

Help is built-in!

- `SessionProbe --help` - outputs the help.

# How to Use ⚙

```text
Usage:
    SessionProbe [flags]

Flags:
  -cookie  string   Session cookie to be used in the requests (required)
  -urls    string   File containing the URLs to be checked (required)
  -threads int      Number of threads (default: 10)
  -out     string   Output file (default: output.txt)

Examples:
    ./SessionProbe -urls "urls.txt" -threads 15 -cookie ".AspNetCore.Cookies=<cookie>" -out output.txt
    ./SessionProbe -urls "urls.txt" -cookie ".AspNetCore.Cookies=<cookie>"
```
