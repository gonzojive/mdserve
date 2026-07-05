# MDServe

MDServe is a developer preview tool written in Go for serving local directories of Markdown files. It supports GitHub Flavored Markdown (GFM) styling, Mermaid diagram rendering, syntax highlighting, and live reloading.

![MDServe Screenshot](screenshot.png)

---

## Features

- 📁 **Directory Serving**: Serves local folders recursively. Automatically renders `README.md` or `index.md` inside directories, or generates a file explorer index.
- 🎨 **GFM Styling**: Renders Markdown with styling inspired by GitHub Pages.
- 📊 **Mermaid Diagrams**: Renders diagrams (such as flowcharts and sequence diagrams) from standard Markdown code blocks.
- ⚡ **Live Reloading**: Refreshes the browser via Server-Sent Events (SSE) when a `.md` file or asset changes.
- 🔍 **File Search & Explorer**: Includes a sidebar file tree with search filtering.
- 🌓 **Light/Dark Mode**: Supports light and dark themes for the page layout, code syntax highlighting, and Mermaid diagrams.
- 🔗 **Clean URLs**: Maps clean URLs (e.g., `/docs/install` maps to `/docs/install.md`).

---

## Quick Start

### 1. Build the Binary
```bash
go build -o mdserve main.go
```

### 2. Start Serving
Run the server pointing to a directory (defaults to current directory `.`):
```bash
./mdserve -dir=. -port=8080
```

### 3. Open in Browser
Visit [http://localhost:8080](http://localhost:8080) to view the rendered documentation.

---

## Installation to User Bin (`~/bin`)
To make the tool globally accessible:
```bash
mkdir -p ~/bin
go build -o ~/bin/mdserve main.go
```
Ensure `~/bin` is in your `PATH` environment variable.

---

## Directory Layout Example

Browse these test files to see nesting and folder routing in action:
- [Installation Guide](docs/installation.md)
- [GFM Tables & Tasklists](docs/tables.md)
- [Mermaid Diagrams Preview](docs/diagrams.md)

---

## Configuration Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-dir` | `.` | Directory containing Markdown files to serve |
| `-port` | `8080` | Port to start the web server on |

---

## License

This project is licensed under the Apache License, Version 2.0. See the [LICENSE](LICENSE) file for details.
