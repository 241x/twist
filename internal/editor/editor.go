package editor

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
)

//go:embed index.html
var assets embed.FS

type Server struct {
	port    int
	openURL string
}

func NewServer(port int) *Server {
	return &Server{port: port}
}

func (s *Server) Start() error {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data, _ := assets.ReadFile("index.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})

	mux.HandleFunc("/api/load", s.handleLoad)
	mux.HandleFunc("/api/save", s.handleSave)

	addr := fmt.Sprintf("127.0.0.1:%d", s.port)
	go func() {
		openBrowser("http://" + addr)
	}()

	return http.ListenAndServe(addr, mux)
}

func (s *Server) handleLoad(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) handleSave(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	defer r.Body.Close()

	var config map[string]any
	if err := json.Unmarshal(body, &config); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	name := "twist.json"
	if n, ok := config["name"].(string); ok && n != "" {
		name = n + ".json"
	}

	pretty, _ := json.MarshalIndent(config, "", "  ")
	if err := os.WriteFile(name, pretty, 0644); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"ok","file":%q}`, name)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	cmd.Start()
}
