# Installation & Usage

This guide describes how to install, build, and use the `mdserve` tool.

## Prerequisites

Before building `mdserve`, you must have Go installed. We recommend Go 1.20 or newer.

Check your Go version:
```bash
go version
```

## Installation

You can build the binary from source in just a few steps:

1. Clone or navigate to the source directory:
   ```bash
   cd busy-faraday
   ```
2. Download dependencies:
   ```bash
   go mod download
   ```
3. Compile the server:
   ```bash
   go build -o mdserve main.go
   ```

## Usage

Start the dev server by running:
```bash
./mdserve
```

By default, it will watch the current directory and start a web server on port `8080`.

### Custom Port and Directory

You can customize the port and target directory using command line flags:

```bash
# Serve files from a "docs" folder on port 9000
./mdserve -dir=./docs -port=9000
```

### Hot Reloading

When `mdserve` is running, it monitors all `.md` files in the target directory recursively. Whenever you save a change to any Markdown file in your text editor, the web page will **automatically refresh** to display the updated content.

> [!TIP]
> The live reload uses Server-Sent Events (SSE). You can verify the status by looking at the green dot in the top-right of the sidebar. If it turns orange, it means the server is down or restarting. Once the server is up again, it will reconnect and reload automatically!
