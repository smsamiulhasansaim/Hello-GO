package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
)

const (
	alphabet     = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	codeLength   = 7
	maxAttempts  = 5
	maxBodyBytes = 1 << 20
)

type Server struct {
	store   *Store
	baseURL string
}

type shortenRequest struct {
	URL string `json:"url"`
}

type shortenResponse struct {
	Code     string `json:"code"`
	ShortURL string `json:"short_url"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func NewServer(store *Store, baseURL string) *Server {
	return &Server{store: store, baseURL: baseURL}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /shorten", s.handleShorten)
	mux.HandleFunc("GET /stats/{code}", s.handleStats)
	mux.HandleFunc("GET /{code}", s.handleRedirect)
	return mux
}

func (s *Server) handleShorten(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	var req shortenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{"invalid JSON body"})
		return
	}
	if !isValidURL(req.URL) {
		writeJSON(w, http.StatusBadRequest, errorResponse{"url must be a valid http or https URL"})
		return
	}

	for range maxAttempts {
		code, err := generateCode(codeLength)
		if err != nil {
			slog.Error("code generation failed", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{"internal error"})
			return
		}

		err = s.store.Save(code, req.URL)
		if errors.Is(err, ErrCodeExists) {
			continue
		}
		if err != nil {
			slog.Error("save failed", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{"internal error"})
			return
		}

		slog.Info("link created", "code", code)
		writeJSON(w, http.StatusCreated, shortenResponse{
			Code:     code,
			ShortURL: s.baseURL + "/" + code,
		})
		return
	}

	slog.Error("exhausted code generation attempts")
	writeJSON(w, http.StatusInternalServerError, errorResponse{"could not generate a unique code"})
}

func (s *Server) handleRedirect(w http.ResponseWriter, r *http.Request) {
	target, ok := s.store.Visit(r.PathValue("code"))
	if !ok {
		writeJSON(w, http.StatusNotFound, errorResponse{"short code not found"})
		return
	}
	// 302 so browsers do not cache the redirect and skip click counting.
	http.Redirect(w, r, target, http.StatusFound)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	link, ok := s.store.Stats(r.PathValue("code"))
	if !ok {
		writeJSON(w, http.StatusNotFound, errorResponse{"short code not found"})
		return
	}
	writeJSON(w, http.StatusOK, link)
}

func isValidURL(raw string) bool {
	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func generateCode(n int) (string, error) {
	max := big.NewInt(int64(len(alphabet)))
	buf := make([]byte, n)
	for i := range buf {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		buf[i] = alphabet[idx.Int64()]
	}
	return string(buf), nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Error("response encoding failed", "error", err)
	}
}
