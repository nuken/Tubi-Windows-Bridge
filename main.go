package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"Tubi_Windows/scraper" // Ensure this matches your go.mod module name
	"golang.org/x/sys/windows/svc"
)

type Config struct {
	Port         int  `json:"port"`
	UseGracenote bool `json:"use_gracenote"`
}

var (
	configLock   sync.RWMutex
	appConfig    Config
	configFile   string
	cacheLock    sync.RWMutex
	channelCache []scraper.ChannelData
	httpServer   *http.Server
	sigMu        sync.Mutex
	cancelCtx    context.CancelFunc
)

func init() {
	exePath, err := os.Executable()
	if err != nil {
		configFile = "settings.json"
		return
	}
	configFile = filepath.Join(filepath.Dir(exePath), "settings.json")
}

func main() {
	// Detect if launched by Windows Service Manager or run manually in a console
	isService, err := svc.IsWindowsService()
	if err != nil {
		log.Fatalf("Failed to determine session type: %v", err)
	}

	if isService {
		runService("TubiBridge")
	} else {
		runInteractive()
	}
}

func runInteractive() {
	ctx, cancel := context.WithCancel(context.Background())
	cancelCtx = cancel

	log.Println("Starting Tubi Bridge...")
	loadConfig()

	// Start Background Scraper
	go epgScheduler(ctx)

	targetPort := appConfig.Port
	if targetPort <= 0 {
		targetPort = 7778
	}

	listener, assignedPort, err := getAvailableListener(targetPort)
	if err != nil {
		log.Fatalf("Failed to bind to an available port: %v", err)
	}

	if appConfig.Port != assignedPort {
		appConfig.Port = assignedPort
		saveConfig()
		log.Printf("Port updated to %d and saved", assignedPort)
	}

	localIP := getLocalIPAddress()
	log.Printf("Dashboard active at: http://%s:%d", localIP, assignedPort)

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleDashboard(localIP, assignedPort))
	mux.HandleFunc("/playlist.m3u", handlePlaylist(localIP, assignedPort))
	mux.HandleFunc("/epg.xml", handleEPG)
	mux.HandleFunc("/api/refresh", handleRefresh)
	mux.HandleFunc("/api/settings", handleSettings)

	httpServer = &http.Server{
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	go func() {
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Wait for an OS interrupt or a shutdown command from service.go
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	select {
	case <-stopChan:
		log.Println("OS Interrupt received.")
	case <-ctx.Done():
		log.Println("Service shutdown requested.")
	}

	log.Println("Shutting down HTTP server...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if httpServer != nil {
		_ = httpServer.Shutdown(shutdownCtx)
	}
}

// --- Background Task ---

func epgScheduler(ctx context.Context) {
	refreshData() // Run immediately on startup

	ticker := time.NewTicker(4 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			refreshData()
		case <-ctx.Done():
			return
		}
	}
}

func refreshData() {
	log.Println("[INFO] Fetching latest Tubi channel and EPG data...")
	data, err := scraper.FetchChannels()
	if err != nil {
		log.Printf("[ERROR] Fetching data: %v", err)
		return
	}
	cacheLock.Lock()
	channelCache = data
	cacheLock.Unlock()
	log.Printf("[INFO] Successfully cached %d channels.", len(data))
}

// --- Dynamic Route Handlers ---

func handlePlaylist(ip string, port int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cacheLock.RLock()
		channels := channelCache
		cacheLock.RUnlock()

		configLock.RLock()
		useGracenote := appConfig.UseGracenote
		configLock.RUnlock()

		baseURL := fmt.Sprintf("http://%s:%d", ip, port)
		m3u := scraper.GenerateM3U(channels, useGracenote, baseURL)

		w.Header().Set("Content-Type", "audio/x-mpegurl")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(m3u))
	}
}

func handleEPG(w http.ResponseWriter, r *http.Request) {
	cacheLock.RLock()
	channels := channelCache
	cacheLock.RUnlock()

	configLock.RLock()
	useGracenote := appConfig.UseGracenote
	configLock.RUnlock()

	xmlBytes, err := scraper.GenerateXMLTV(channels, useGracenote)
	if err != nil {
		http.Error(w, "Failed to generate EPG", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(xmlBytes)
}

func handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	go refreshData()
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"refresh_triggered"}`))
}

func handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload struct {
		UseGracenote *bool `json:"use_gracenote"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	configLock.Lock()
	if payload.UseGracenote != nil {
		appConfig.UseGracenote = *payload.UseGracenote
	}
	saveConfigLocked()
	configLock.Unlock()

	// Clear cache and force an immediate refresh to apply the new setting
	go refreshData()

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"saved"}`))
}

// --- Configuration Helpers ---

func loadConfig() {
	configLock.Lock()
	defer configLock.Unlock()

	appConfig = Config{
		Port:         7778,
		UseGracenote: true,
	}

	if data, err := os.ReadFile(configFile); err == nil {
		if err := json.Unmarshal(data, &appConfig); err != nil {
			log.Printf("Warning: Corrupted %s, falling back to defaults.", configFile)
		}
	} else {
		// First-run: write defaults to disk
		saveConfigLocked()
	}
}

func saveConfig() {
	configLock.Lock()
	defer configLock.Unlock()
	saveConfigLocked()
}

func saveConfigLocked() {
	data, err := json.MarshalIndent(appConfig, "", "  ")
	if err != nil {
		log.Printf("Error serializing config: %v", err)
		return
	}
	_ = os.WriteFile(configFile, data, 0644)
}

// --- Networking & Port Discovery ---

func getAvailableListener(startingPort int) (net.Listener, int, error) {
	// 1. Try startingPort and the next 100 ports
	for port := startingPort; port < startingPort+100; port++ {
		addr := fmt.Sprintf("0.0.0.0:%d", port)
		listener, err := net.Listen("tcp", addr)
		if err == nil {
			return listener, port, nil
		}
	}

	// 2. Fall back to letting the OS kernel assign any free ephemeral port
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		return nil, 0, fmt.Errorf("no available ports found: %w", err)
	}

	assignedPort := listener.Addr().(*net.TCPAddr).Port
	return listener, assignedPort, nil
}

func getLocalIPAddress() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err == nil {
		defer conn.Close()
		localAddr := conn.LocalAddr().(*net.UDPAddr)
		return localAddr.IP.String()
	}

	addrs, err := net.InterfaceAddrs()
	if err == nil {
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					return ipnet.IP.String()
				}
			}
		}
	}
	return "127.0.0.1"
}

// --- Dashboard ---

func handleDashboard(ip string, port int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		configLock.RLock()
		gracenoteActive := appConfig.UseGracenote
		configLock.RUnlock()

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <title>Tubi Bridge</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #1a1a1a; color: #fff; padding: 2rem; margin: 0; }
        .card { max-width: 600px; margin: 0 auto; background: #242424; padding: 24px; border-radius: 8px; box-shadow: 0 4px 6px rgba(0,0,0,0.3); }
        h1 { margin-top: 0; font-size: 20px; }
        .link-row { display: flex; align-items: center; justify-content: space-between; margin-bottom: 12px; background: #161616; padding: 10px; border-radius: 4px; }
        a { color: #58a6ff; text-decoration: none; font-family: monospace; }
        button { background: #238636; color: #fff; border: 0; padding: 6px 12px; border-radius: 4px; cursor: pointer; }
        button:hover { background: #2ea043; }
        .setting-box { margin-top: 20px; padding-top: 15px; border-top: 1px solid #333; }
    </style>
</head>
<body>
    <div class="card">
        <h1>Tubi Bridge Dashboard v1.0.0</h1>
        <div class="link-row">
            <div>
                <div><strong>M3U Playlist</strong></div>
                <a href="http://%s:%d/playlist.m3u">http://%s:%d/playlist.m3u</a>
            </div>
            <button onclick="copyToClipboard('http://%s:%d/playlist.m3u')">Copy</button>
        </div>
        <div class="link-row">
            <div>
                <div><strong>XMLTV Guide</strong></div>
                <a href="http://%s:%d/epg.xml">http://%s:%d/epg.xml</a>
            </div>
            <button onclick="copyToClipboard('http://%s:%d/epg.xml')">Copy</button>
        </div>
        <div class="setting-box">
            <label>
                <input type="checkbox" id="useGracenote" %s onchange="toggleGracenote(this.checked)">
                Use Gracenote Station IDs (Recommended for Channels DVR)
            </label>
        </div>
    </div>
    <script>
        function copyToClipboard(text) {
            const textarea = document.createElement('textarea');
            textarea.value = text;
            textarea.style.position = 'fixed';
            textarea.style.left = '-9999px';
            document.body.appendChild(textarea);
            textarea.select();
            document.execCommand('copy');
            document.body.removeChild(textarea);
            alert('Copied to clipboard!');
        }
        function toggleGracenote(enabled) {
            fetch('/api/settings', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ use_gracenote: enabled })
            }).then(() => location.reload());
        }
    </script>
</body>
</html>`, ip, port, ip, port, ip, port, ip, port, ip, port, ip, port, checkedAttr(gracenoteActive))
	}
}

func checkedAttr(b bool) string {
	if b {
		return "checked"
	}
	return ""
}