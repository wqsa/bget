package bget

import (
	"sync"
	"time"
)

const hostoryTime = 5 * time.Second

type dataStats struct {
	mu              sync.Mutex
	download        int64
	historyDownload map[time.Time]int64
	upload          int64
	historyUpload   map[time.Time]int64
	duration        time.Duration
	lastActive      time.Time
}

func (s *dataStats) resetTimer() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastActive = time.Now()
}

func (s *dataStats) addDownload(value int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.download += value
	clearExpiredValue(s.historyDownload)
	s.historyDownload[time.Now()] = value
}

func (s *dataStats) addUpload(value int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.upload += value
	clearExpiredValue(s.historyUpload)
	s.historyUpload[time.Now()] = value
}

func (s *dataStats) downloadSpeed() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.speed(s.historyDownload)
}

func (s *dataStats) uploadSpeed() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.speed(s.historyUpload)
}

func (s *dataStats) speed(values map[time.Time]int64) float64 {
	clearExpiredValue(values)
	var l int64
	for _, v := range values {
		l += v
	}
	return float64(l) / hostoryTime.Seconds()
}

func clearExpiredValue(values map[time.Time]int64) {
	for t := range values {
		if time.Since(t) > hostoryTime {
			delete(values, t)
		}
	}
}

type torrentStats struct {
}
