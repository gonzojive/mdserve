# MDServe 🚀

Welcome to **MDServe**, a lightweight, high-performance developer tool written in Go for serving and rendering local directories of Markdown files with all the power of GitHub Pages, built-in Mermaid diagram support, syntax highlighting, and instant live reloading!

## Features

- 📁 **Seamless Directory Serving**: Serves any local folder recursively. Automatically renders `README.md` / `index.md` inside directories, or generates a clean file-explorer index.
- 🎨 **GitHub Pages Look & Feel**: Uses CSS styling styled exactly like GitHub Pages.
- 📊 **Mermaid Diagrams**: Native support for rendering graphical diagrams (`flowchart`, `sequenceDiagram`, `gantt`, etc.) from standard Markdown code blocks.
- ⚡ **Live Reloading**: Instant browser refresh via Server-Sent Events (SSE) when any `.md` file or local image/style changes. No plugin required!
- 🔍 **File Search & Explorer**: An interactive, collapsible sidebar file explorer with real-time fuzzy filter search.
- 🌓 **Sleek Light/Dark Mode**: Smooth transition between light and dark themes that updates the markdown container, sidebar, code highlighting, and Mermaid graphs.
- 🔗 **Extensionless Clean URLs**: Clean route mapping (e.g. `/docs/install` translates automatically to `/docs/install.md`).

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
Visit [http://localhost:8080](http://localhost:8080) to view your rendered documentation.

---

## Directory Layout Example

To see nesting and folder routing in action, expand the **docs** folder in the sidebar on the left or browse these test files:
- [Installation Guide](/docs/installation)
- [GFM Tables & Tasklists](/docs/tables)
- [Mermaid Diagrams Preview](/docs/diagrams)

---

## Configuration Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-dir` | `.` | Directory containing Markdown files to serve |
| `-port` | `8080` | Port to start the web server on |

---

## Markdown & HTML Live Demo

Here is a quick check of standard GFM formatting:
- [x] Task lists are fully supported
- [x] Strikethroughs (e.g. ~~old features~~) work
- [x] Autolinks (e.g. https://github.com) work
- [x] Beautiful tables are rendered correctly

Enjoy writing and previewing your markdown locally! ✍️
