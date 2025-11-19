# loged

A lightweight, real-time log streaming tool for Linux servers built with AI.

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
- **Basic authentication** - secure access with username/password
- **Pagination** - loads last 200 lines by default, load more on demand
- **Nginx reverse proxy** - automatic setup with SSL-ready configuration

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
- **Nginx** (automatically installed on Linux)

## Installation

### One-Command Installation (Linux/macOS/WSL)

```bash
git clone <repo-url> && cd loged && ./loged
```

This will automatically:
- Install Go if needed
- Install nginx if needed (Linux only)
- Build the application
- Configure nginx reverse proxy
- Set up everything for production use

### Manual Installation

1. **Install Go 1.21+** from https://golang.org/dl/
2. **Clone and setup:**
   ```bash
   git clone <repo-url>
   cd loged
   ./loged install
   ```

### Windows Installation

1. **Install Go** from https://golang.org/dl/ (download the .msi installer)
2. **Clone and build:**
   ```bash
   git clone <repo-url>
   cd loged
   go mod tidy
   go build -o runtime/loged-server.exe src/main.go
   ```

## Usage

### Commands

```bash
./loged install    # Install dependencies and build (one-time setup)
./loged start      # Start server in background
./loged stop       # Stop server
./loged status     # Check if server is running
./loged update     # Stop, rebuild, and restart server
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
    path: "./src/sample.log"
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

#### Direct Access
1. **Start the server:**
   ```bash
   ./loged start
   ```

2. **Open your browser to:** `http://localhost:8008`

#### Via Nginx Reverse Proxy (Linux)
1. **Add to /etc/hosts:**
   ```
   127.0.0.1 loged.logs
   ```

2. **Access via:** `http://loged.logs/loaded`

### Using the Interface

1. **Login with credentials** (if authentication is enabled)

2. **Select log files:**
   - Choose from configured log files on the homepage
   - Or enter a custom log file path
   - Each log file opens in its own view

3. **Real-time monitoring:**
   - Logs stream in real-time as new lines are appended
   - Shows last 200 lines by default
   - Click "Load 100 More Lines" to see older entries
   - Auto-scroll keeps latest entries visible
   - Open multiple tabs for different log files

### Direct URL Access

Bookmark or share direct links to specific log files:

**Direct access:**
```
http://localhost:8008?file=/var/log/syslog
http://localhost:8008?file=/var/log/nginx/access.log
```

**Via nginx proxy:**
```
http://loged.logs/loaded?file=/var/log/syslog
http://loged.logs/loaded?file=/var/log/nginx/access.log
```

### Command Line Options

```bash
./runtime/loged-server -port 8080    # Override config port (manual run)
```

## Production Deployment

### Security Recommendations
1. **Change default credentials** in config.yml
2. **Use HTTPS** - Configure SSL certificate in nginx
3. **Restrict file access** - Only allow specific log directories
4. **Firewall rules** - Limit access to specific IPs
5. **Run as dedicated user** with minimal permissions

### SSL/HTTPS Setup
The nginx configuration is ready for SSL. Add your certificates:

```nginx
server {
    listen 443 ssl;
    server_name loged.logs;
    
    ssl_certificate /path/to/your/certificate.crt;
    ssl_certificate_key /path/to/your/private.key;
    
    location /loaded {
        # existing proxy configuration
    }
}
```

## File Structure

```
loged/
├── README.md          # This documentation
├── LICENSE            # License file
├── config.yml         # Configuration file
├── loged              # Installation and control script
├── loged-nginx        # Nginx reverse proxy configuration
├── .gitignore         # Git ignore rules
├── src/               # Source code directory
│   ├── main.go        # Complete server implementation
│   ├── go.mod         # Go module dependencies
│   ├── go.sum         # Go dependency checksums
│   └── sample.log     # Test log file
└── runtime/           # Generated files (created automatically)
    ├── loged-server   # Built binary
    ├── loged.pid      # Process ID file (when running)
    └── loged.log      # Server logs (when running)
```

## License

Open source - feel free to modify and distribute.
