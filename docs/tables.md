# GFM Tables & Formatting

This page serves as a test file for GitHub Flavored Markdown (GFM) features.

## Table Test

Below is a standard GFM Markdown table. It should render with proper borders, spacing, alternating row backgrounds (zebra striping), and alignment.

| Name | Role | Status | Favorite Language | Commits |
| :--- | :---: | :---: | :--- | ---: |
| Alice | Core Dev | 🟢 Active | Go | 412 |
| Bob | Frontend Dev | 🟡 Idle | TypeScript | 230 |
| Charlie | Designer | 🔴 Away | HTML/CSS | 45 |
| Dave | Devops | 🟢 Active | Rust | 189 |

## Task Lists

Task lists should render as checkbox elements.

- [x] Initial design proposal
- [x] Backend HTTP server in Go
- [x] HTML & CSS templates
- [x] Live reload (SSE)
- [ ] Production build optimization
- [ ] Custom styling themes support

## Strikethrough & Autolinks

- **Strikethrough**: This is a ~~deprecated statement~~ that is crossed out.
- **Autolinks**: The parser should automatically detect raw URLs and convert them to links, such as https://pkg.go.dev/github.com/yuin/goldmark or mailto:developer@example.com.

## Blockquotes with GitHub Alerts

Alerts are rendered using GitHub-style blockquote decorators.

> [!NOTE]
> This is a note alert for informative messages.

> [!TIP]
> This is a tip alert for recommendations and best practices.

> [!IMPORTANT]
> This is an important alert for vital instructions.

> [!WARNING]
> This is a warning alert for actions that need caution.

> [!CAUTION]
> This is a caution alert for potentially destructive actions.
