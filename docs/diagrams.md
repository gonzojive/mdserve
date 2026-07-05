# Mermaid Diagrams Preview

This page tests the integration of **Mermaid.js** within Markdown files. Code blocks with the language set to `mermaid` will be automatically rendered as SVG diagrams in the browser.

## 1. Flowchart Example

Here is a standard flowchart showing the lifecycle of a request in `mdserve`:

```mermaid
graph TD
    User([User Request]) --> Route{Clean Route}
    Route -- Directory --> HasReadme{Has README.md?}
    HasReadme -- Yes --> RenderMD[Render README.md]
    HasReadme -- No --> RenderDir[Render File Index]
    Route -- File .md --> RenderMD
    Route -- Other File --> ServeAsset[Serve Static File]
    Route -- Not Found --> Serve404[Serve 404 Page]
    
    RenderMD --> HTML[Wrap in HTML Template]
    RenderDir --> HTML
    HTML --> Response([HTTP Response])
    ServeAsset --> Response
    Serve404 --> Response
```

## 2. Sequence Diagram Example

This sequence diagram illustrates the live reloading flow via Server-Sent Events (SSE):

```mermaid
sequenceDiagram
    participant Browser as Client Browser
    participant Server as Go HTTP Server
    participant Watcher as fsnotify Watcher
    participant Editor as Text Editor

    Browser->>Server: Connect to /events (SSE)
    Server-->>Browser: Connection established (status: connected)
    Note over Browser, Server: Keeps connection open in background
    
    rect rgb(88, 166, 255, 0.1)
        Editor->>Watcher: Modify and Save document.md
        Watcher->>Server: File Write event detected
        Note over Server: Debounce for 200ms
        Server->>Browser: Broadcast "reload" event
    end
    
    Browser->>Server: Request updated page content
    Server-->>Browser: Serve newly rendered HTML
    Browser->>Browser: Reload page
```

## 3. Git Graph Example

Testing Git branching logic using Mermaid:

```mermaid
gitGraph
    commit
    commit
    branch develop
    checkout develop
    commit id: "add GFM support"
    commit id: "add fsnotify watcher"
    checkout main
    merge develop
    commit id: "release v1.0.0"
    branch hotfix
    checkout hotfix
    commit id: "fix sse disconnects"
    checkout main
    merge hotfix
```

Verify that all diagrams render graphical SVGs and change their themes correctly when switching between Light and Dark mode!
