package main

import (
	"context"
	"sync"
)

type connectionSupervisor struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	gen    uint64
}

func (s *connectionSupervisor) Start(parent context.Context) (context.Context, uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if parent == nil {
		parent = context.Background()
	}
	if s.cancel != nil {
		s.cancel()
	}
	ctx, cancel := context.WithCancel(parent)
	s.gen++
	s.cancel = cancel
	return ctx, s.gen
}

func (s *connectionSupervisor) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.gen++
}

func (s *connectionSupervisor) IsCurrent(gen uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return gen != 0 && s.gen == gen && s.cancel != nil
}
