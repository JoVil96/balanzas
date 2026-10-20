package scale

import (
	"context"
	"sync"

	"github.com/jovil96/balanza-agent/internal/domain"
)

type Source interface{ Ch() <-chan domain.Reading }
type Store interface{ SaveReading(domain.Reading) error }

// Service owns the single serial stream and distributes each reading to all consumers.
type Service struct {
	source    Source
	store     Store
	mu        sync.RWMutex
	latest    domain.Reading
	hasLatest bool
	subs      map[chan domain.Reading]struct{}
}

func New(source Source, store Store) *Service {
	return &Service{source: source, store: store, subs: make(map[chan domain.Reading]struct{})}
}

func (s *Service) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case reading := <-s.source.Ch():
				_ = s.store.SaveReading(reading)
				s.mu.Lock()
				s.latest, s.hasLatest = reading, true
				for ch := range s.subs {
					select {
					case ch <- reading:
					default:
						select {
						case <-ch:
						default:
						}
						select {
						case ch <- reading:
						default:
						}
					}
				}
				s.mu.Unlock()
			}
		}
	}()
}

func (s *Service) Latest() (domain.Reading, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.latest, s.hasLatest
}

func (s *Service) Subscribe() (<-chan domain.Reading, func()) {
	ch := make(chan domain.Reading, 1)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	latest, ok := s.latest, s.hasLatest
	s.mu.Unlock()
	if ok {
		ch <- latest
	}
	return ch, func() { s.mu.Lock(); delete(s.subs, ch); close(ch); s.mu.Unlock() }
}
