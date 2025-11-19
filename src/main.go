package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/gorilla/websocket"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Port     int `yaml:"port"`
	Auth     struct {
		Enabled  bool   `yaml:"enabled"`
		Username string `yaml:"username"`
		Password string `yaml:"password"`
	} `yaml:"auth"`
	LogFiles []struct {
		Path string `yaml:"path"`
		Name string `yaml:"name"`
	} `yaml:"log_files"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type LogStreamer struct {
	clients  []*websocket.Conn
	filename string
	mutex    sync.Mutex
}

func NewLogStreamer(filepath string) (*LogStreamer, error) {
	if _, err := os.Stat(filepath); os.IsNotExist(err) {
		return nil, err
	}
	
	return &LogStreamer{
		clients:  make([]*websocket.Conn, 0),
		filename: filepath,
	}, nil
}

func (ls *LogStreamer) AddClient(conn *websocket.Conn) {
	ls.mutex.Lock()
	ls.clients = append(ls.clients, conn)
	ls.mutex.Unlock()
	
	// Send last 200 lines initially
	go func() {
		file, err := os.Open(ls.filename)
		if err != nil {
			return
		}
		defer file.Close()
		
		// Read all lines first
		var lines []string
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		
		// Send last 200 lines
		start := 0
		if len(lines) > 200 {
			start = len(lines) - 200
		}
		
		for i := start; i < len(lines); i++ {
			conn.WriteMessage(websocket.TextMessage, []byte(lines[i]))
		}
		
		// Send initial line count
		totalLines := len(lines)
		shownLines := len(lines) - start
		conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("__META__:INITIAL_LOAD:%d:%d", totalLines, shownLines)))
	}()
}

func (ls *LogStreamer) RemoveClient(conn *websocket.Conn) {
	ls.mutex.Lock()
	for i, client := range ls.clients {
		if client == conn {
			ls.clients = append(ls.clients[:i], ls.clients[i+1:]...)
			break
		}
	}
	ls.mutex.Unlock()
	conn.Close()
}

func (ls *LogStreamer) Broadcast(message string) {
	ls.mutex.Lock()
	for i := len(ls.clients) - 1; i >= 0; i-- {
		client := ls.clients[i]
		err := client.WriteMessage(websocket.TextMessage, []byte(message))
		if err != nil {
			client.Close()
			ls.clients = append(ls.clients[:i], ls.clients[i+1:]...)
		}
	}
	ls.mutex.Unlock()
}

func (ls *LogStreamer) Start() {
	go func() {
		file, err := os.Open(ls.filename)
		if err != nil {
			return
		}
		defer file.Close()
		
		file.Seek(0, 2) // Go to end
		scanner := bufio.NewScanner(file)
		
		for scanner.Scan() {
			ls.Broadcast(scanner.Text())
		}
	}()
}

var streamers = make(map[string]*LogStreamer)
var config Config

func requireAuth(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !config.Auth.Enabled {
			handler(w, r)
			return
		}
		
		username, password, ok := r.BasicAuth()
		if !ok || username != config.Auth.Username || password != config.Auth.Password {
			w.Header().Set("WWW-Authenticate", `Basic realm="Loged"`)
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head><title>Authentication Required</title>
<style>
body { font-family: Arial, sans-serif; background: #1e1e1e; color: #fff; text-align: center; padding: 50px; }
h1 { color: #2196F3; }
</style>
</head>
<body>
<h1>Authentication Required</h1>
<p>Please provide valid credentials to access the log viewer.</p>
</body>
</html>`)
			return
		}
		
		handler(w, r)
	}
}

func loadConfig() error {
	data, err := os.ReadFile("config.yml")
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, &config)
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	logPath := r.URL.Query().Get("file")
	if logPath == "" {
		http.Error(w, "file parameter required", http.StatusBadRequest)
		return
	}

	log.Printf("WebSocket request for file: %s", logPath)

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	streamer, exists := streamers[logPath]
	if !exists {
		streamer, err = NewLogStreamer(logPath)
		if err != nil {
			log.Printf("Error creating streamer for %s: %v", logPath, err)
			conn.WriteMessage(websocket.TextMessage, []byte("Error: "+err.Error()))
			conn.Close()
			return
		}
		streamers[logPath] = streamer
		streamer.Start()
		log.Printf("Started streaming for: %s", logPath)
	}

	streamer.AddClient(conn)
	log.Printf("Client connected for file: %s", logPath)

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Client disconnected: %v", err)
			streamer.RemoveClient(conn)
			break
		}
		
		// Handle load more requests
		if string(message) == "LOAD_MORE" {
			go func() {
				file, err := os.Open(logPath)
				if err != nil {
					return
				}
				defer file.Close()
				
				var lines []string
				scanner := bufio.NewScanner(file)
				for scanner.Scan() {
					lines = append(lines, scanner.Text())
				}
				
				conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("__META__:LOAD_MORE_RESPONSE:%d", len(lines))))
				
				// Send 100 more lines from the requested position
				// This will be handled by the frontend
			}()
		}
	}
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	logPath := r.URL.Query().Get("file")
	log.Printf("HTTP request for path: %s, file param: %s", r.URL.Path, logPath)
	
	if logPath == "" {
		// Show available log files from config
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head><title>Loged - Log Viewer</title>
<style>
* { box-sizing: border-box; }
body { 
    font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; 
    margin: 0; padding: 0; 
    background: linear-gradient(135deg, #1e1e1e 0%%, #2d2d2d 100%%); 
    color: #e0e0e0; 
    min-height: 100vh;
}
.container { max-width: 800px; margin: 0 auto; padding: 40px 20px; }
h1 { 
    color: #2196F3; 
    text-align: center; 
    margin-bottom: 40px; 
    font-size: 36px; 
    font-weight: 300;
    text-shadow: 0 2px 4px rgba(0,0,0,0.3);
}
.section { 
    background: rgba(255,255,255,0.05); 
    margin: 30px 0; 
    padding: 25px; 
    border-radius: 12px; 
    border: 1px solid rgba(33, 150, 243, 0.2);
    box-shadow: 0 4px 15px rgba(0,0,0,0.2);
}
.section h3 { 
    color: #2196F3; 
    margin-top: 0; 
    font-size: 20px; 
    font-weight: 500;
}
.log-item { 
    margin: 15px 0; 
    padding: 15px; 
    background: rgba(255,255,255,0.08); 
    border-radius: 8px; 
    border-left: 4px solid #2196F3;
    transition: all 0.3s ease;
}
.log-item:hover { 
    background: rgba(33, 150, 243, 0.1); 
    transform: translateX(5px);
}
.log-item a { 
    color: #2196F3; 
    text-decoration: none; 
    font-weight: 500; 
    font-size: 16px;
}
.log-item a:hover { color: #42A5F5; }
.log-item small { 
    color: #aaa; 
    font-size: 13px; 
    display: block; 
    margin-top: 5px;
}
.custom-form { 
    display: flex; 
    gap: 10px; 
    align-items: center; 
    flex-wrap: wrap;
}
.custom-form input { 
    padding: 12px 15px; 
    flex: 1; 
    min-width: 300px; 
    background: rgba(255,255,255,0.1); 
    border: 1px solid #555; 
    border-radius: 6px; 
    color: #fff; 
    font-size: 14px;
}
.custom-form input:focus { 
    outline: none; 
    border-color: #2196F3; 
    box-shadow: 0 0 0 2px rgba(33, 150, 243, 0.2);
}
.custom-form button { 
    padding: 12px 20px; 
    background: #2196F3; 
    color: white; 
    border: none; 
    border-radius: 6px; 
    cursor: pointer; 
    font-weight: 500;
    transition: background 0.3s;
}
.custom-form button:hover { background: #1976D2; }
.empty-state { 
    text-align: center; 
    color: #888; 
    font-style: italic; 
    padding: 20px;
}
</style>
</head>
<body>
<div class="container">
<h1>Loged - Real-time Log Viewer</h1>
<div class="section">
<h3>Available Log Files</h3>`)
		
		hasFiles := false
		for _, logFile := range config.LogFiles {
			if _, err := os.Stat(logFile.Path); err == nil {
				fmt.Fprintf(w, `<div class="log-item"><a href="?file=%s">%s</a><small>%s</small></div>`, logFile.Path, logFile.Name, logFile.Path)
				hasFiles = true
			}
		}
		
		if !hasFiles {
			fmt.Fprintf(w, `<div class="empty-state">No log files found. Check your config.yml or add a custom path below.</div>`)
		}
		
		fmt.Fprintf(w, `</div>
<div class="section">
<h3>Custom Log File</h3>
<form class="custom-form">
<input type="text" name="file" placeholder="/path/to/your/log/file" required>
<button type="submit">View Log</button>
</form>
</div>
</div>
</body>
</html>`)
		return
	}

	// Check if file exists
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		http.Error(w, "File not found: "+logPath, http.StatusNotFound)
		return
	}

	filename := filepath.Base(logPath)
	log.Printf("Serving log viewer for file: %s", logPath)
	
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head>
<title>%s - Loged</title>
<style>
* { box-sizing: border-box; }
body { 
    font-family: 'Consolas', 'Monaco', 'Courier New', monospace; 
    margin: 0; padding: 0; 
    background: linear-gradient(135deg, #1e1e1e 0%%, #2d2d2d 100%%); 
    color: #e0e0e0; 
    height: 100vh;
}
.header { 
    background: #333; 
    padding: 15px 20px; 
    border-bottom: 2px solid #2196F3; 
    box-shadow: 0 2px 10px rgba(0,0,0,0.3);
}
.back-link { 
    color: #2196F3; 
    text-decoration: none; 
    margin-right: 20px; 
    font-weight: bold;
    transition: color 0.3s;
}
.back-link:hover { color: #42A5F5; }
h1 { 
    color: #2196F3; 
    margin: 0; 
    display: inline-block; 
    font-size: 24px;
}
#status { 
    color: #888; 
    margin: 10px 0; 
    font-size: 14px;
    padding: 8px 12px;
    background: rgba(255,255,255,0.05);
    border-radius: 4px;
    display: inline-block;
}
.container { 
    padding: 20px; 
    height: calc(100vh - 80px); 
    display: flex; 
    flex-direction: column;
}
.log-controls {
    margin-bottom: 10px;
    display: flex;
    align-items: center;
    gap: 15px;
}
#loadMoreBtn {
    background: #2196F3;
    color: white;
    border: none;
    padding: 8px 16px;
    border-radius: 4px;
    cursor: pointer;
    font-size: 12px;
    transition: background 0.3s;
}
#loadMoreBtn:hover { background: #1976D2; }
#loadMoreBtn:disabled { 
    background: #555; 
    cursor: not-allowed; 
}
.log-info {
    color: #888;
    font-size: 12px;
}
#logs { 
    background: #000; 
    padding: 15px; 
    flex: 1; 
    overflow-y: auto; 
    border: 1px solid #444; 
    border-radius: 8px;
    box-shadow: inset 0 2px 10px rgba(0,0,0,0.5);
    font-size: 13px;
    line-height: 1.4;
}
.log-line { 
    margin: 3px 0; 
    padding: 2px 0;
    border-left: 3px solid transparent;
    padding-left: 8px;
    transition: all 0.2s;
}
.log-line:hover {
    background: rgba(33, 150, 243, 0.1);
    border-left-color: #2196F3;
}
.log-line.new {
    animation: highlight 0.5s ease-out;
}
@keyframes highlight {
    0%% { background: rgba(33, 150, 243, 0.3); }
    100%% { background: transparent; }
}
::-webkit-scrollbar { width: 8px; }
::-webkit-scrollbar-track { background: #2d2d2d; }
::-webkit-scrollbar-thumb { background: #2196F3; border-radius: 4px; }
::-webkit-scrollbar-thumb:hover { background: #42A5F5; }
</style>
</head>
<body>
<div class="header">
    <a href="/" class="back-link">Back to Log List</a>
    <h1>%s</h1>
    <div id="status">Connecting...</div>
</div>
<div class="container">
    <div class="log-controls">
        <button id="loadMoreBtn" onclick="loadMore()">Load 100 More Lines</button>
        <span class="log-info" id="logInfo">Loading...</span>
    </div>
    <div id="logs"></div>
</div>
<script>
console.log('Connecting to WebSocket...');
const ws = new WebSocket('ws://'+location.host+'/ws?file=%s');
const logs = document.getElementById('logs');
const status = document.getElementById('status');
const loadMoreBtn = document.getElementById('loadMoreBtn');
const logInfo = document.getElementById('logInfo');

let totalLines = 0;
let shownLines = 0;
let allLines = [];

ws.onopen = function() {
    console.log('WebSocket connected');
    status.textContent = 'Connected - Monitoring log file';
    status.style.color = '#2196F3';
};

ws.onmessage = function(event) {
    const data = event.data;
    
    // Handle metadata messages
    if (data.startsWith('__META__:')) {
        const parts = data.split(':');
        if (parts[1] === 'INITIAL_LOAD') {
            totalLines = parseInt(parts[2]);
            shownLines = parseInt(parts[3]);
            updateLogInfo();
            return;
        }
        if (parts[1] === 'LOAD_MORE_RESPONSE') {
            totalLines = parseInt(parts[2]);
            return;
        }
    }
    
    // Regular log line
    const line = document.createElement('div');
    line.className = 'log-line new';
    line.textContent = data;
    
    // If it's a new real-time log (not from load more)
    if (!data.startsWith('__HISTORICAL__:')) {
        logs.appendChild(line);
        logs.scrollTop = logs.scrollHeight;
        shownLines++;
        totalLines++;
        updateLogInfo();
    } else {
        // Historical line from load more
        const content = data.substring('__HISTORICAL__:'.length);
        line.textContent = content;
        line.classList.remove('new');
        logs.insertBefore(line, logs.firstChild);
        shownLines++;
        updateLogInfo();
    }
    
    // Remove animation class after animation completes
    setTimeout(() => line.classList.remove('new'), 500);
};

ws.onclose = function() {
    console.log('WebSocket closed');
    status.textContent = 'Connection closed';
    status.style.color = '#f44336';
};

ws.onerror = function(error) {
    console.error('WebSocket error:', error);
    status.textContent = 'Connection error';
    status.style.color = '#f44336';
};

function loadMore() {
    if (shownLines >= totalLines) return;
    
    loadMoreBtn.disabled = true;
    loadMoreBtn.textContent = 'Loading...';
    
    // Request more lines from server
    fetch('/api/loadmore?file=%s&offset=' + (totalLines - shownLines - 100) + '&limit=100')
        .then(response => response.json())
        .then(data => {
            const scrollPos = logs.scrollTop;
            const scrollHeight = logs.scrollHeight;
            
            data.lines.forEach(lineText => {
                const line = document.createElement('div');
                line.className = 'log-line';
                line.textContent = lineText;
                logs.insertBefore(line, logs.firstChild);
            });
            
            shownLines += data.lines.length;
            
            // Maintain scroll position
            logs.scrollTop = scrollPos + (logs.scrollHeight - scrollHeight);
            
            updateLogInfo();
            loadMoreBtn.disabled = false;
            loadMoreBtn.textContent = 'Load 100 More Lines';
        })
        .catch(error => {
            console.error('Load more failed:', error);
            loadMoreBtn.disabled = false;
            loadMoreBtn.textContent = 'Load 100 More Lines';
        });
}

function updateLogInfo() {
    logInfo.textContent = 'Showing ' + shownLines + ' of ' + totalLines + ' lines';
    loadMoreBtn.style.display = shownLines >= totalLines ? 'none' : 'inline-block';
}
</script>
</body>
</html>`, filename, filename, logPath)
}

func handleLoadMore(w http.ResponseWriter, r *http.Request) {
	logPath := r.URL.Query().Get("file")
	offsetStr := r.URL.Query().Get("offset")
	limitStr := r.URL.Query().Get("limit")
	
	if logPath == "" {
		http.Error(w, "file parameter required", http.StatusBadRequest)
		return
	}
	
	offset := 0
	limit := 100
	
	if offsetStr != "" {
		fmt.Sscanf(offsetStr, "%d", &offset)
	}
	if limitStr != "" {
		fmt.Sscanf(limitStr, "%d", &limit)
	}
	
	file, err := os.Open(logPath)
	if err != nil {
		http.Error(w, "Cannot open file", http.StatusInternalServerError)
		return
	}
	defer file.Close()
	
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	
	// Calculate range
	start := offset
	if start < 0 {
		start = 0
	}
	end := start + limit
	if end > len(lines) {
		end = len(lines)
	}
	
	var result []string
	for i := start; i < end; i++ {
		result = append(result, lines[i])
	}
	
	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"lines": result,
		"total": len(lines),
		"offset": start,
		"limit": limit,
	}
	
	json.NewEncoder(w).Encode(response)
}

func main() {
	port := flag.String("port", "", "Port to run server on (overrides config)")
	flag.Parse()

	// Load configuration
	if err := loadConfig(); err != nil {
		log.Printf("Warning: Could not load config.yml: %v", err)
		config.Port = 8008 // Default port
	}

	// Override port if provided via command line
	if *port != "" {
		fmt.Sscanf(*port, "%d", &config.Port)
	}

	http.HandleFunc("/", requireAuth(handleIndex))
	http.HandleFunc("/ws", requireAuth(handleWebSocket))
	http.HandleFunc("/api/loadmore", requireAuth(handleLoadMore))

	fmt.Printf("Loged server starting on port %d\n", config.Port)
	if config.Auth.Enabled {
		fmt.Printf("Authentication enabled - Username: %s\n", config.Auth.Username)
	} else {
		fmt.Printf("Authentication disabled\n")
	}
	fmt.Printf("Open http://localhost:%d in your browser\n", config.Port)
	
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", config.Port), nil))
}
