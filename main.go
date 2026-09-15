package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"Tubi_Windows/scraper"
	"golang.org/x/sys/windows/svc"
)

type Config struct {
	Port         int    `json:"port"`
	UseGracenote bool   `json:"use_gracenote"`
	Username     string `json:"username"`
	Password     string `json:"password"`
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

func epgScheduler(ctx context.Context) {
	refreshData()
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
	configLock.RLock()
	user := appConfig.Username
	pass := appConfig.Password
	configLock.RUnlock()

	if user == "" || pass == "" {
		log.Println("[WARNING] Tubi credentials not configured. Open the dashboard to save your login.")
		return
	}

	log.Println("[INFO] Fetching latest Tubi channel and EPG data...")
	data, err := scraper.FetchChannels(user, pass)
	if err != nil {
		log.Printf("[ERROR] Fetching data: %v", err)
		return
	}

	cacheLock.Lock()
	channelCache = data
	cacheLock.Unlock()
	log.Printf("[INFO] Successfully cached %d channels.", len(data))
}

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
		UseGracenote *bool   `json:"use_gracenote"`
		Username     *string `json:"username"`
		Password     *string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	configLock.Lock()
	if payload.UseGracenote != nil {
		appConfig.UseGracenote = *payload.UseGracenote
	}
	if payload.Username != nil {
		appConfig.Username = *payload.Username
	}
	if payload.Password != nil {
		appConfig.Password = *payload.Password
	}
	saveConfigLocked()
	configLock.Unlock()

	go refreshData()
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"saved"}`))
}

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

func getAvailableListener(startingPort int) (net.Listener, int, error) {
	for port := startingPort; port < startingPort+100; port++ {
		addr := fmt.Sprintf("0.0.0.0:%d", port)
		listener, err := net.Listen("tcp", addr)
		if err == nil {
			return listener, port, nil
		}
	}

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

func handleDashboard(ip string, port int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		configLock.RLock()
		gracenoteActive := appConfig.UseGracenote
		user := appConfig.Username
		pass := appConfig.Password
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
        .form-group { margin-bottom: 12px; }
        .form-group label { display: block; font-size: 13px; margin-bottom: 4px; color: #bbb; }
        .form-group input[type="text"], .form-group input[type="password"] { width: 100%%; box-sizing: border-box; padding: 8px; border-radius: 4px; border: 1px solid #444; background: #161616; color: #fff; }
    </style>
</head>
<body>
    <div class="card">
        <h1>Tubi Bridge Dashboard v1.0.1</h1>
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
            <h3>Tubi Credentials</h3>
            <div class="form-group">
                <label>Email</label>
                <input type="text" id="username" value="%s" placeholder="email@example.com">
            </div>
            <div class="form-group">
                <label>Password</label>
                <input type="password" id="password" value="%s">
            </div>
            <div style="margin-top: 10px; margin-bottom: 15px;">
                <label>
                    <input type="checkbox" id="useGracenote" %s>
                    Use Gracenote Station IDs (Channels DVR)
                </label>
            </div>
            <button onclick="saveSettings()">Save & Refresh</button>
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
        function saveSettings() {
            const username = document.getElementById('username').value;
            const password = document.getElementById('password').value;
            const useGracenote = document.getElementById('useGracenote').checked;
            fetch('/api/settings', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ username: username, password: password, use_gracenote: useGracenote })
            }).then(() => {
                alert('Settings saved. Refresh triggered!');
                location.reload();
            });
        }
    </script>
</body>
</html>`, ip, port, ip, port, ip, port, ip, port, ip, port, ip, port, user, pass, checkedAttr(gracenoteActive))
	}
}

func checkedAttr(b bool) string {
	if b {
		return "checked"
	}
	return ""
}