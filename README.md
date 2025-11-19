<div align="center">
  <img src="src/catlog.png" alt="Catlog Logo" width="120" height="120">
</div>

# catlog

A lightweight, real-time log streaming tool for Linux servers built with AI.

## About

Catlog is an open-source, minimal log viewer that runs entirely on your server without requiring external backends or databases. It provides a web-based interface for monitoring log files in real-time, accessible from any browser.

### Features
- **Real-Time Log Streaming** via Secure WebSocket Connections
- **Multi-User Authentication** with role-based access control
- **Web-based Interface** accessible from any browser
- **Multiple Log Files Support** - each file opens in separately
- **Minimal Resource Footprint** - single Go binary, ~1000 lines of code
- **No External Dependencies** - no databases or centralized services required
- **Configuration-Driven** - manage users and log files via single configuration file
- **Supports Pagination** - loads last 200 lines by default, load more on demand
- **Nginx with SSL** - automatic setup with SSL-ready configuration
- **Rate Limiting** - protection against excessive requests and web crawlers/bots
- **Custom Timezone Support** - logs displayed in configured timezone
- **Auto-start on Boot** - automatically restarts after server reboot (Linux)

### Supported Operating Systems
- Linux

### Stack
- Go
- Nginx

### Target Audience
- Developers and DevOps engineers needing quick log visibility
- System administrators managing small to medium servers
- Teams using low-resource environments where Elastic Stack is overkill
- Anyone wanting simple, fast log monitoring without heavy setup

## Installation

### One-Command Installation and Startup (Linux/macOS/WSL)

```bash
git clone https://github.com/rutwikdeshmukh/catlog && cd catlog && chmod +x catlog && ./catlog install  && ./catlog start
```

This will automatically:
- Install Go if needed
- Install nginx if needed (Linux only)
- Build the application
- Configure nginx reverse proxy with rate limiting
- Set up IST timezone
- **Configure auto-start on server reboot (Linux only)**
- Set up everything for production use

### Manual Installation

1. **Install Go 1.21+** from https://golang.org/dl/
2. **Run:**
   ```bash
   git clone https://github.com/rutwikdeshmukh/catlog
   cd catlog
   chmod +x catlog
   ./catlog install
   ```

<!-- ### Windows Installation

1. **Install Go** from https://golang.org/dl/ (download the .msi installer)
2. **Clone and build:**
   ```bash
   git clone https://github.com/rutwikdeshmukh/catlog
   cd catlog
   go mod tidy
   go build -o runtime/catlog-server.exe src/main.go
   ``` -->

## Usage

### Commands

```bash
./catlog install    # Install dependencies and build (one-time setup)
./catlog start      # Start server in background
./catlog stop       # Stop server
./catlog status     # Check if server is running
./catlog update     # Stop, rebuild, and restart server
./catlog restart    # Restart with IP detection and SSL regeneration
./catlog uninstall  # Remove all catlog files and configurations
```

### Configuration

Edit `config.yml` to customize users, log files, port, and authentication:

```yaml
# Configuration file for the log monitoring application
port: 8008
server_ip: "127.0.0.1"  # Auto-updated by restart command
timezone: "Asia/Kolkata"  # IST timezone for logs and timestamps
ssl:
  enabled: true
  cert_path: "/etc/ssl/certs/catlog.crt"
  key_path: "/etc/ssl/private/catlog.key"
auth:
  enabled: true
  users:
    - username: "admin"
      password: "catlog123"
      role: "admin"  # admin has access to all logs
    - username: "BEDeveloper"
      password: "backend123"
      role: "backend"
      allowed_paths:
        - "/var/log/supervisor/*"
        - "/var/log/app/*"
        - "./runtime/catlog.log"
    - username: "FEDeveloper"
      password: "frontend123"
      role: "frontend"
      allowed_paths:
        - "/var/log/nginx/*"
        - "/var/log/apache2/*"
    - username: "DevOps"
      password: "devops123"
      role: "devops"
      allowed_paths:
        - "/var/log/syslog"
        - "/var/log/auth.log"
        - "/var/log/nginx/*"
log_files:
  - name: "Sample Log"
    path: "./src/sample.log"
  - name: "System Log"
    path: "/var/log/syslog"
  - name: "Nginx Access Log"
    path: "/var/log/nginx/catlog_access.log"
  - name: "Nginx SSL Access Log"
    path: "/var/log/nginx/catlog_ssl_access.log"
  - name: "Nginx Error Log"
    path: "/var/log/nginx/error.log"
  - name: "Catlog Log"
    path: "./runtime/catlog.log"
```

### RBAC
- `admin` role has access to all logs
- Other roles have restricted access based on `allowed_paths` in `config.yml`
- Supports wildcard patterns (e.g., `/var/log/nginx/*`)
- Session-based authentication with login/logout

**Note:** Change the default credentials before deploying to production.

### Server Restart Handling

The `restart` command automatically handles server IP changes:

```bash
./catlog restart    # Detects new IP, regenerates SSL, updates nginx
```

**What it does:**
- Detects current public IP address
- Updates `server_ip` in config.yml
- Regenerates SSL certificates with new IP
- Updates nginx configuration
- Reloads nginx and restarts catlog

**Auto-start on boot:**
Auto-start is automatically configured during installation. If you need to set it up manually:
```bash
crontab -e
@reboot sleep 60 && cd /path/to/catlog && ./catlog restart
```

### Rate Limiting
Automatic rate limiting is configured:
- General endpoints: 30 requests/minute
- API endpoints: 10 requests/minute
- Burst protection included

### Accessing the Interface

1. **Visit login page:** `https://<your-server-ip>/login`
2. **Login with credentials** from config.yml
3. **Browse available logs** based on your user permissions
4. **Click logout** for clean session termination

### Using the Interface

1. **Login** with your username and password
2. **Select log files** from your available list (filtered by permissions)
3. **Real-time monitoring:**
   - Logs stream in real-time
   - Shows last 200 lines by default
   - Click "Load 100 More Lines" to see older entries
   - Error keywords highlighted in red
   - Open multiple tabs for different log files
4. **Logout** cleanly using the logout button

## Production Deployment

### Security Recommendations
1. **Change Default Credentials** in config.yml to **Stronger Credentials**
2. **Use HTTPS** - Use domain specific SSL certificates generated by a trrusted CA
3. **Configure User Permissions** - Restrict access to specific log directories
4. **Firewall Rules** - Limit access to specific IPs
5. **Run as Dedicated User** with minimal permissions

## File Structure

```
catlog/
├── README.md          # Documentation for this project
├── LICENSE            # License
├── config.yml         # Configuration file based on example.config.yml
├── example.config.yml # Example configuration
├── catlog              # Installation and Control Script
├── catlog-nginx        # Nginx configuration
├── .gitattributes     # Defines how the contents stored in the repository are copied to the working tree
├── .gitignore         # Git ignore rules
├── src/               # Source code directory
│   ├── main.go        # Complete server implementation
│   ├── go.mod         # Go module dependencies
│   ├── go.sum         # Go dependency checksums
│   └── sample.log     # Test log file
└── runtime/           # Generated files (created automatically)
    ├── catlog-server   # Built binary
    ├── catlog.pid      # Process ID file (when running)
    └── catlog.log      # Server logs (when running)
```
## Total Hours Spent on this Idea
```
18
```
Update the counter like a message smeared on a wall with blood to let others know how much efforts have been made :)
