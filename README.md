# loged

A lightweight, real-time log streaming tool for Linux servers. 
Started with an Idea, built with AI.

## About

Loged is an open-source, minimal log viewer that runs entirely on your server without requiring external backends or databases. It provides a web-based interface for monitoring log files in real-time, similar to `tail -f` but accessible from any browser.

### Features
- **Real-time log streaming** via WebSocket connections
- **Web-based interface** accessible from any browser
- **Multiple log files support** - each file opens in separate tabs/windows
- **Minimal resource footprint** - single Go binary, ~200 lines of code
- **No external dependencies** - no databases or centralized services required
- **Server-side only** - runs where your logs are located
- **Auto-scroll** - new log entries appear automatically
- **Modern UI** - clean, responsive interface with dark theme
- **Configuration-driven** - manage log files via config.yml
- **Background service** - runs as daemon, doesn't occupy terminal

### Target Audience
- Developers and DevOps engineers needing quick log visibility
- System administrators managing small to medium servers
- Teams using low-resource environments where Elastic Stack is overkill
- Anyone wanting simple, fast log monitoring without heavy setup

## Dependencies

- **Go 1.21+** - Install from https://golang.org/dl/
- **Linux/Unix system** with log files
- **Read permissions** for target log files
- **Modern web browser** with WebSocket support

## Installation

### One-Command Installation (Linux/macOS/WSL)

```bash
git clone https://github.com/rutwikdeshmukh/loged.git && cd loged && ./loged
```

### Manual Installation

1. **Install Go 1.21+** from https://golang.org/dl/
2. **Clone and setup:**
   ```bash
   git clone https://github.com/rutwikdeshmukh/loged.git
   cd loged
   ./loged install
   ```

### Windows Installation

1. **Install Go** from https://golang.org/dl/ (download the .msi installer)
2. **Clone and build:**
   ```bash
   git clone https://github.com/rutwikdeshmukh/loged.git
   cd loged
   go mod tidy
   go build -o loged-server.exe main.go
   ```

## Usage

### Commands

```bash
./loged install    # Install dependencies and build (one-time setup)
./loged start      # Start server in background
./loged stop       # Stop server
./loged status     # Check if server is running
```

### Configuration

Edit `config.yml` to customize log files, port, and authentication:

```yaml
# Configuration file for the log monitoring application
port: 8008
auth:
  enabled: true
  username: "admin"
  password: "loged123"
log_files:
  - name: "Sample Log"
    path: "./sample.log"
  - name: "System Log"
    path: "/var/log/syslog"
  - name: "Auth Log"
    path: "/var/log/auth.log"
  - name: "Application Log"
    path: "./app.log"
```

**Authentication Options:**
- `enabled: true/false` - Enable or disable basic authentication
- `username` - Username for accessing the log viewer
- `password` - Password for accessing the log viewer

**Security Note:** Change the default credentials before deploying to production.

### Accessing the Interface

1. **Start the server:**
   ```bash
   ./loged start
   ```

2. **Open your browser to:** `http://localhost:8008`

3. **Select log files:**
   - Choose from configured log files on the homepage
   - Or enter a custom log file path
   - Each log file opens in its own view

4. **Real-time monitoring:**
   - Logs stream in real-time as new lines are appended
   - Auto-scroll keeps latest entries visible
   - Open multiple tabs for different log files

### Direct URL Access

Bookmark or share direct links to specific log files:
```
http://localhost:8008?file=/var/log/syslog
http://localhost:8008?file=/var/log/nginx/access.log
http://localhost:8008?file=/home/user/app.log
```

### Command Line Options

```bash
./loged-server -port 8080    # Override config port (manual run)
```

## File Structure

```
loged/
├── main.go          # Complete server implementation
├── go.mod           # Go module dependencies
├── config.yml       # Configuration file
├── loged            # Installation and control script
├── loged-server     # Built binary (created after installation)
├── sample.log       # Test log file
└── README.md        # This documentation
```

## License

Open source - feel free to modify and distribute.
