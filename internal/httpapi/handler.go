package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"url-shortener/internal/shortener"
	"url-shortener/internal/store/memory"
)

const maxBodyBytes = 8 << 10

type Handler struct {
	store   *memory.Store
	baseURL string
	mux     *http.ServeMux
}

func New(store *memory.Store, baseURL string) *Handler {
	h := &Handler{
		store:   store,
		baseURL: strings.TrimRight(baseURL, "/"),
		mux:     http.NewServeMux(),
	}
	h.mux.HandleFunc("POST /api/shorten", h.shorten)
	h.mux.HandleFunc("GET /{code}", h.redirect)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
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

func (h *Handler) shorten(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)

	var req shortenRequest
	if err := dec.Decode(&req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusBadRequest, "request body too large")
			return
		}
		writeError(w, http.StatusBadRequest, "request body must be a JSON object like {\"url\":\"https://...\"}")
		return
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "request body must contain a single JSON object")
		return
	}

	longURL, err := shortener.NormalizeURL(req.URL)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid url: "+err.Error())
		return
	}

	link, err := h.store.Shorten(longURL)
	if err != nil {
		log.Printf("shorten: %v", err)
		writeError(w, http.StatusInternalServerError, "could not create short link")
		return
	}

	writeJSON(w, http.StatusCreated, shortenResponse{
		Code:     link.Code,
		ShortURL: h.baseURL + "/" + link.Code,
	})
}

func (h *Handler) redirect(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	if !shortener.IsValidCode(code) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	link, ok := h.store.Get(code)
	if !ok {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	// Set Location directly instead of http.Redirect, which may rewrite the
	// target and adds an HTML body.
	w.Header().Set("Location", link.URL)
	w.WriteHeader(http.StatusFound)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}
