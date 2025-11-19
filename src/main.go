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
	"strings"
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
	
	// Get base path for URLs (detect if behind reverse proxy)
	basePath := ""
	originalURI := r.Header.Get("X-Original-URI")
	if originalURI != "" && strings.HasPrefix(originalURI, "/loged") {
		basePath = "/loged"
	}
	
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
    font-family: 'Inter', 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; 
    margin: 0; padding: 0; 
    background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
    color: #ffffff; 
    min-height: 100vh;
    overflow-x: hidden;
}
.container { 
    max-width: 900px; 
    margin: 0 auto; 
    padding: 60px 20px; 
    position: relative;
}
.container::before {
    content: '';
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    background: rgba(255,255,255,0.05);
    backdrop-filter: blur(10px);
    border-radius: 20px;
    z-index: -1;
}
h1 { 
    color: #ffffff; 
    text-align: center; 
    margin-bottom: 50px; 
    font-size: 42px; 
    font-weight: 700;
    text-shadow: 0 4px 20px rgba(0,0,0,0.3);
    background: linear-gradient(45deg, #ff6b6b, #4ecdc4, #45b7d1);
    background-size: 300%% 300%%;
    -webkit-background-clip: text;
    -webkit-text-fill-color: transparent;
    animation: gradient 3s ease infinite;
}
@keyframes gradient {
    0%% { background-position: 0%% 50%%; }
    50%% { background-position: 100%% 50%%; }
    100%% { background-position: 0%% 50%%; }
}
.section { 
    background: linear-gradient(145deg, rgba(255,255,255,0.1), rgba(255,255,255,0.05));
    margin: 30px 0; 
    padding: 30px; 
    border-radius: 20px; 
    border: 1px solid rgba(255,255,255,0.2);
    box-shadow: 0 8px 32px rgba(0,0,0,0.1);
    backdrop-filter: blur(10px);
    transition: transform 0.3s ease, box-shadow 0.3s ease;
}
.section:hover {
    transform: translateY(-5px);
    box-shadow: 0 15px 40px rgba(0,0,0,0.2);
}
.section h3 { 
    color: #ffffff; 
    margin-top: 0; 
    font-size: 24px; 
    font-weight: 600;
    margin-bottom: 25px;
}
.log-item { 
    margin: 20px 0; 
    padding: 20px; 
    background: linear-gradient(135deg, rgba(255,255,255,0.15), rgba(255,255,255,0.05));
    border-radius: 15px; 
    border-left: 5px solid #ff6b6b;
    transition: all 0.3s ease;
    position: relative;
    overflow: hidden;
}
.log-item::before {
    content: '';
    position: absolute;
    top: 0;
    left: -100%%;
    width: 100%%;
    height: 100%%;
    background: linear-gradient(90deg, transparent, rgba(255,255,255,0.1), transparent);
    transition: left 0.5s;
}
.log-item:hover::before {
    left: 100%%;
}
.log-item:hover { 
    background: linear-gradient(135deg, rgba(255,107,107,0.2), rgba(78,205,196,0.2));
    transform: translateX(10px) scale(1.02);
    border-left-color: #4ecdc4;
}
.log-item a { 
    color: #ffffff; 
    text-decoration: none; 
    font-weight: 600; 
    font-size: 18px;
    display: block;
    margin-bottom: 8px;
}
.log-item a:hover { 
    color: #4ecdc4; 
    text-shadow: 0 0 10px rgba(78,205,196,0.5);
}
.log-item small { 
    color: rgba(255,255,255,0.7); 
    font-size: 14px; 
    display: block;
    font-family: 'Courier New', monospace;
}
.custom-form { 
    display: flex; 
    gap: 15px; 
    align-items: center; 
    flex-wrap: wrap;
}
.custom-form input { 
    padding: 15px 20px; 
    flex: 1; 
    min-width: 300px; 
    background: rgba(255,255,255,0.1); 
    border: 2px solid rgba(255,255,255,0.2); 
    border-radius: 12px; 
    color: #fff; 
    font-size: 16px;
    backdrop-filter: blur(10px);
    transition: all 0.3s ease;
}
.custom-form input::placeholder {
    color: rgba(255,255,255,0.6);
}
.custom-form input:focus { 
    outline: none; 
    border-color: #4ecdc4; 
    box-shadow: 0 0 20px rgba(78,205,196,0.3);
    background: rgba(255,255,255,0.15);
}
.custom-form button { 
    padding: 15px 30px; 
    background: linear-gradient(45deg, #ff6b6b, #4ecdc4); 
    color: white; 
    border: none; 
    border-radius: 12px; 
    cursor: pointer; 
    font-weight: 600;
    font-size: 16px;
    transition: all 0.3s ease;
    box-shadow: 0 4px 15px rgba(0,0,0,0.2);
}
.custom-form button:hover { 
    transform: translateY(-2px);
    box-shadow: 0 8px 25px rgba(0,0,0,0.3);
    background: linear-gradient(45deg, #4ecdc4, #ff6b6b);
}
.empty-state { 
    text-align: center; 
    color: rgba(255,255,255,0.7); 
    font-style: italic; 
    padding: 30px;
    font-size: 18px;
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
				fmt.Fprintf(w, `<div class="log-item"><a href="%s?file=%s">%s</a><small>%s</small></div>`, basePath, logFile.Path, logFile.Name, logFile.Path)
				hasFiles = true
			}
		}
		
		if !hasFiles {
			fmt.Fprintf(w, `<div class="empty-state">No log files found. Check your config.yml or add a custom path below.</div>`)
		}
		
		fmt.Fprintf(w, `</div>
<div class="section">
<h3>Custom Log File</h3>
<form class="custom-form" action="%s">
<input type="text" name="file" placeholder="/path/to/your/log/file" required>
<button type="submit">View Log</button>
</form>
</div>
</div>
</body>
</html>`, basePath)
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
    font-family: 'Inter', 'Consolas', 'Monaco', 'Courier New', monospace; 
    margin: 0; padding: 0; 
    background: linear-gradient(135deg, #2c3e50 0%%, #34495e 50%%, #2c3e50 100%%);
    color: #ecf0f1; 
    height: 100vh;
    overflow: hidden;
}
.header { 
    background: linear-gradient(135deg, rgba(52,73,94,0.95), rgba(44,62,80,0.95));
    padding: 20px 30px; 
    border-bottom: 3px solid #e74c3c;
    box-shadow: 0 4px 20px rgba(0,0,0,0.3);
    backdrop-filter: blur(10px);
    position: relative;
}
.header::before {
    content: '';
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    height: 3px;
    background: linear-gradient(90deg, #e74c3c, #f39c12, #27ae60, #3498db, #9b59b6);
    background-size: 300%% 100%%;
    animation: rainbow 3s linear infinite;
}
@keyframes rainbow {
    0%% { background-position: 0%% 50%%; }
    100%% { background-position: 300%% 50%%; }
}
.back-link { 
    color: #3498db; 
    text-decoration: none; 
    margin-right: 25px; 
    font-weight: 600;
    font-size: 16px;
    transition: all 0.3s ease;
    padding: 8px 16px;
    border-radius: 8px;
    background: rgba(52,152,219,0.1);
}
.back-link:hover { 
    color: #ffffff; 
    background: rgba(52,152,219,0.2);
    transform: translateX(-3px);
    box-shadow: 0 4px 12px rgba(52,152,219,0.3);
}
h1 { 
    color: #ecf0f1; 
    margin: 0; 
    display: inline-block; 
    font-size: 28px;
    font-weight: 700;
    text-shadow: 0 2px 10px rgba(0,0,0,0.5);
}
#status { 
    color: #bdc3c7; 
    margin: 15px 0 0 0; 
    font-size: 14px;
    padding: 10px 15px;
    background: rgba(255,255,255,0.1);
    border-radius: 8px;
    display: inline-block;
    backdrop-filter: blur(5px);
    border: 1px solid rgba(255,255,255,0.1);
}
.container { 
    padding: 25px; 
    height: calc(100vh - 120px); 
    display: flex; 
    flex-direction: column;
}
.log-controls {
    margin-bottom: 15px;
    display: flex;
    align-items: center;
    gap: 20px;
    padding: 15px 20px;
    background: rgba(255,255,255,0.05);
    border-radius: 12px;
    backdrop-filter: blur(10px);
}
#loadMoreBtn {
    background: linear-gradient(45deg, #e74c3c, #c0392b);
    color: white;
    border: none;
    padding: 12px 24px;
    border-radius: 8px;
    cursor: pointer;
    font-size: 14px;
    font-weight: 600;
    transition: all 0.3s ease;
    box-shadow: 0 4px 15px rgba(231,76,60,0.3);
}
#loadMoreBtn:hover { 
    background: linear-gradient(45deg, #c0392b, #e74c3c);
    transform: translateY(-2px);
    box-shadow: 0 6px 20px rgba(231,76,60,0.4);
}
#loadMoreBtn:disabled { 
    background: linear-gradient(45deg, #7f8c8d, #95a5a6);
    cursor: not-allowed; 
    transform: none;
    box-shadow: none;
}
.log-info {
    color: #bdc3c7;
    font-size: 14px;
    font-weight: 500;
}
#logs { 
    background: linear-gradient(145deg, rgba(0,0,0,0.4), rgba(0,0,0,0.2));
    padding: 20px; 
    flex: 1; 
    overflow-y: auto; 
    border: 1px solid rgba(255,255,255,0.1); 
    border-radius: 12px;
    box-shadow: inset 0 4px 20px rgba(0,0,0,0.3);
    font-size: 14px;
    line-height: 1.6;
    backdrop-filter: blur(5px);
}
.log-line { 
    margin: 4px 0; 
    padding: 8px 12px;
    border-left: 3px solid transparent;
    border-radius: 6px;
    transition: all 0.2s ease;
    font-family: 'Fira Code', 'Consolas', monospace;
}
.log-line:hover {
    background: rgba(52,152,219,0.1);
    border-left-color: #3498db;
    transform: translateX(5px);
}
.log-line.new {
    animation: logHighlight 0.8s ease-out;
}
@keyframes logHighlight {
    0%% { 
        background: rgba(46,204,113,0.4);
        border-left-color: #2ecc71;
        transform: translateX(10px) scale(1.02);
    }
    100%% { 
        background: transparent;
        border-left-color: transparent;
        transform: translateX(0) scale(1);
    }
}
::-webkit-scrollbar { width: 12px; }
::-webkit-scrollbar-track { 
    background: rgba(255,255,255,0.05); 
    border-radius: 6px;
}
::-webkit-scrollbar-thumb { 
    background: linear-gradient(45deg, #e74c3c, #c0392b);
    border-radius: 6px;
    border: 2px solid rgba(255,255,255,0.1);
}
::-webkit-scrollbar-thumb:hover { 
    background: linear-gradient(45deg, #c0392b, #e74c3c);
}
</style>
</head>
<body>
<div class="header">
    <a href="%s" class="back-link">Back to Log List</a>
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
const wsProtocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
const basePath = '%s';
const wsPath = basePath ? basePath + '/ws' : '/ws';
const ws = new WebSocket(wsProtocol + '//' + location.host + wsPath + '?file=%s');
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
    const apiPath = basePath ? basePath + '/api/loadmore' : '/api/loadmore';
    fetch(apiPath + '?file=%s&offset=' + (totalLines - shownLines - 100) + '&limit=100')
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
</html>`, filename, basePath, filename, basePath, logPath, logPath)
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
