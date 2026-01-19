# fif

A TUI (Terminal User Interface) application that provides a **Find in Files** experience similar to JetBrains IDEs.

Search, preview, and open results in your editor directly from the terminal, without depending on VS Code / Cursor extensions.

## Features

- 🚀 **Fast Search**: High-speed full-text search using ripgrep
- 📝 **Incremental Search**: Real-time search results as you type
- 👀 **Preview**: Preview surrounding code for selected results
- 🔍 **Scope Switching**: Search in the entire project or current directory
- 📁 **File Masking**: Filter search targets using glob patterns
- 🎨 **JetBrains-like UI**: Familiar interface

## Requirements

- Go 1.25.5 or higher
- [ripgrep](https://github.com/BurntSushi/ripgrep) (`rg`) installed and available in PATH
- VS Code or Cursor (for opening results)

## Installation

### Build and Install

```bash
git clone https://github.com/takaishi/fif.git
cd fif
make install
```

Or

```bash
go install github.com/takaishi/fif@latest
```

### Build Only

```bash
make build
```

The built binary will be generated as `./fif`.

## Usage

### Basic Usage

```bash
fif
```

After launching, you can:

1. **Enter search query**: Type the string you want to search (incremental search)
2. **Select results**: Use ↑↓ keys or j/k keys to navigate results
3. **Preview**: Surrounding code for the selected result is automatically displayed
4. **Open in editor**: Press Enter to open the selected result in your editor
5. **Exit**: Press Esc or Ctrl+C to exit

### Command Line Options

```bash
fif --editor cursor  # Use Cursor
fif --editor code    # Use VS Code
```

### Environment Variables

```bash
export FIF_EDITOR=cursor  # Set default editor to Cursor
fif
```

Editor priority:
1. Editor specified by `--editor` flag
2. `FIF_EDITOR` environment variable
3. Auto-detection (tries `cursor` → `code` in order)

## Key Bindings

| Key | Action |
|-----|--------|
| ↑ / ↓ | Navigate up/down in results list |
| j / k | Vim-style navigation |
| Enter | Open selected result in editor |
| Tab | Switch between query input and file mask input |
| Alt+P | Switch to project scope (when in Git repository) |
| Alt+D | Switch to directory scope |
| Esc / Ctrl+C | Exit |

## UI Layout

```
┌ Find in Files: <query>     [x] File mask: <glob> ┐
├───────────────────────────────────────────────────┤
│ Scope: [In Project]  In Directory                │
├───────────────────────────────────────────────────┤
│ Results                                           │
│ > code snippet | file.go:42                       │
│   another match | file2.go:18                     │
│   ...                                            │
├───────────────────────────────────────────────────┤
│ Preview                                           │
│ file.go                                           │
│                                                   │
│   37 | func main() {                             │
│   38 |   foo := 1                                │
│   39 |   if err != nil {                         │
│   40 |     panic(err)                            │
└───────────────────────────────────────────────────┘
Status: 100+ matches in 41+ files
```

## Features

### Search Scope

- **In Project**: Search the entire Git repository root directory
- **In Directory**: Search only the current working directory

When launched inside a Git repository, the default is "In Project".

### File Mask

Filter search targets using glob patterns.

Examples:
- `*.go` - Go files only
- `*.{ts,tsx}` - TypeScript files only
- `!*.test.go` - Exclude test files

You can toggle the mask on/off using the checkbox.

### Preview

The surrounding lines (before and after) of the selected search result are automatically displayed in the preview. The matched line is highlighted.

## Development

### Project Structure

```
fif/
  main.go              # Entry point
  config/              # Configuration management
  editor/              # Editor launching
  preview/             # Preview functionality
  search/              # Search functionality (ripgrep integration)
  tui/                 # TUI implementation
  docs/                # Documentation
```

### Build

```bash
make build
```

### Test

```bash
make test
```

### Format Code

```bash
make fmt
```

### Lint

```bash
make lint
```

### Other Commands

```bash
make help  # Show available commands
```

## Technology Stack

- **Language**: Go 1.25.5
- **TUI Framework**: [tview](https://github.com/rivo/tview)
- **Search Engine**: [ripgrep](https://github.com/BurntSushi/ripgrep)
- **Editor Integration**: VS Code / Cursor `--goto` option

## License

(Add license information here)

## Contributing

Pull requests and issue reports are welcome.

## Related Links

- [Design Document](docs/design.md)
