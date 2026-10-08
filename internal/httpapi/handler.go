package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"url-shortener/internal/shortener"
)

const maxBodyBytes = 8 << 10

type Handler struct {
	store   Store
	baseURL string
	mux     *http.ServeMux
}

func New(store Store, baseURL string) *Handler {
	h := &Handler{
		store:   store,
		baseURL: strings.TrimRight(baseURL, "/"),
		mux:     http.NewServeMux(),
	}
	h.mux.HandleFunc("POST /api/shorten", h.shorten)
	h.mux.HandleFunc("GET /api/v1/links/{code}", h.metadata)
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

type linkResponse struct {
	URL       string `json:"url"`
	CreatedAt string `json:"created_at"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func (h *Handler) shorten(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)

	var req shortenRequest
	if err := dec.Decode(&req); err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			writeError(w, http.StatusRequestTimeout, "request body was not received in time")
			return
		}
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
		writeDomainError(w, err)
		return
	}

	link, err := h.store.Shorten(longURL)
	if err != nil {
		writeDomainError(w, fmt.Errorf("shorten: %w", err))
		return
	}

	writeJSON(w, http.StatusCreated, shortenResponse{
		Code:     link.Code,
		ShortURL: h.baseURL + "/" + link.Code,
	})
}

func (h *Handler) redirect(w http.ResponseWriter, r *http.Request) {
	link, err := h.lookup(r.PathValue("code"))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	// Set Location directly instead of http.Redirect, which may rewrite the
	// target and adds an HTML body.
	w.Header().Set("Location", link.URL)
	w.WriteHeader(http.StatusFound)
}

func (h *Handler) metadata(w http.ResponseWriter, r *http.Request) {
	link, err := h.lookup(r.PathValue("code"))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, linkResponse{
		URL:       link.URL,
		CreatedAt: link.CreatedAt.UTC().Format(time.RFC3339),
	})
}

func (h *Handler) lookup(code string) (shortener.Link, error) {
	if !shortener.IsValidCode(code) {
		return shortener.Link{}, fmt.Errorf("code %q: %w", code, shortener.ErrNotFound)
	}
	return h.store.Get(code)
}

func writeDomainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, shortener.ErrInvalidURL):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, shortener.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, shortener.ErrStoreFull):
		log.Printf("store full: %v", err)
		writeError(w, http.StatusInsufficientStorage, "link storage is full")
	default:
		log.Printf("internal error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
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
