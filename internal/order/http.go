package order

import (
	"encoding/json"
	"net/http"
	"strings"
)

type HTTPServer struct {
	controller *Controller
}

func NewHTTPServer(controller *Controller) http.Handler {
	s := &HTTPServer{controller: controller}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.health)
	mux.HandleFunc("/orders", s.orders)
	mux.HandleFunc("/bots", s.bots)
	mux.HandleFunc("/status", s.status)
	mux.HandleFunc("/finalize", s.finalize)
	return mux
}

func (s *HTTPServer) health(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *HTTPServer) orders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	orderType := NormalOrder
	if strings.EqualFold(r.URL.Query().Get("type"), "vip") {
		orderType = VIPOrder
	}
	writeJSON(w, http.StatusCreated, s.controller.CreateOrder(orderType))
}

func (s *HTTPServer) bots(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		writeJSON(w, http.StatusCreated, s.controller.AddBot())
	case http.MethodDelete:
		bot, err := s.controller.RemoveNewestBot()
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, http.StatusOK, bot)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *HTTPServer) status(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.controller.Snapshot())
}

func (s *HTTPServer) finalize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.controller.WriteFinalStatus()
	writeJSON(w, http.StatusOK, s.controller.Snapshot())
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
