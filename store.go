package main

import (
	"errors"
	"sync"
	"time"
)

var ErrCodeExists = errors.New("short code already exists")

type Link struct {
	URL       string    `json:"url"`
	Clicks    int64     `json:"clicks"`
	CreatedAt time.Time `json:"created_at"`
}

type Store struct {
	mu    sync.RWMutex
	links map[string]*Link
}

func NewStore() *Store {
	return &Store{links: make(map[string]*Link)}
}

func (s *Store) Save(code, rawURL string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.links[code]; exists {
		return ErrCodeExists
	}
	s.links[code] = &Link{URL: rawURL, CreatedAt: time.Now().UTC()}
	return nil
}

// Visit returns the original URL and records the click atomically.
func (s *Store) Visit(code string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	link, ok := s.links[code]
	if !ok {
		return "", false
	}
	link.Clicks++
	return link.URL, true
}

func (s *Store) Stats(code string) (Link, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	link, ok := s.links[code]
	if !ok {
		return Link{}, false
	}
	return *link, true
}