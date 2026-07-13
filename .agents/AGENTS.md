# mdserve Project Customization Rules

These guidelines are loaded and enforced for all AI coding agents working on the `mdserve` project to maintain code quality, readability, and consistency.

## Go Code Quality & Design Standards

### 1. Uber Go Style Guide
All Go code must conform to the **Uber Go Style Guide**. Key highlights to check and enforce:
- **Early Returns**: Avoid deeply nested code; return errors or exit early.
- **Error Formatting**: Error messages must be lowercased and should not end with punctuation (e.g., `fmt.Errorf("error resolving path: %w", err)`).
- **Interface Segregation**: Return concrete structures from functions, but accept interfaces where appropriate (e.g. `io.Reader`). Avoid pointers to interfaces.
- **Mutex Zero-Value**: Initialize mutexes as zero-values; do not use pointers to mutexes unless strictly necessary.
- **Consistent Receivers**: Struct method receivers must be consistent. If any method of a struct uses a pointer receiver (`*T`), all methods must use pointer receivers.

### 2. Mandatory Documentation
- Every public (exported) symbol (functions, methods, structs, interfaces, variables, constants) must have a clear Go doc comment.

### 3. Readability & Function Length
- Readability is prioritized over clever or overly optimized code.
- Keep functions short and focused. **Functions must not exceed 50-80 lines**.
- If a function grows larger, split it into smaller, well-named helper functions.

### 4. Unit Testing
- Every new feature, command, or critical helper must have comprehensive unit tests.
- Mock external dependencies (like filesystems or HTTP clients) where appropriate, or use temporary directories (`t.TempDir()`) to verify filesystem side-effects.
