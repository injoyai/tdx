// Package testutil provides database-free fixtures for package tests.
package testutil

import (
	"errors"
	"sort"
	"strconv"
	"sync"
)

type Marks struct {
	Mu     sync.Mutex
	Marks  map[string]int64
	Err    error
	Closed bool
}

func (s *Marks) LoadUpdate(k string) (int64, bool, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	v, ok := s.Marks[k]
	return v, ok, s.Err
}
func (s *Marks) SaveUpdate(k string, v int64) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	if s.Err != nil {
		return s.Err
	}
	if s.Marks == nil {
		s.Marks = map[string]int64{}
	}
	s.Marks[k] = v
	return nil
}
func (s *Marks) Close() error { s.Closed = true; return nil }

// Pull is a test fixture parameterized by the owning package's data models.
// It never imports pull, so internal pull tests can also use it without a cycle.
type Pull[C comparable, D, M, Cov, Scan any] struct {
	Marks
	Namespace  string
	Key        func(C) string
	DayTime    func(*D) string
	MinuteTime func(*M) string
	Merge      func(*Cov, *Cov) *Cov
	days       map[string][]*D
	minutes    map[string][]*M
	coverage   map[string]*Cov
	scans      map[string]*Scan
}

func (s *Pull[C, D, M, Cov, Scan]) Identity() string         { return s.Namespace }
func (s *Pull[C, D, M, Cov, Scan]) minKey(c C, y int) string { return s.Key(c) + ":" + strconv.Itoa(y) }
func (s *Pull[C, D, M, Cov, Scan]) QueryDay(c C, a, b string) ([]*D, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	return filter(s.days[s.Key(c)], a, b, s.DayTime), s.Err
}
func (s *Pull[C, D, M, Cov, Scan]) QueryMinute(c C, a, b string) ([]*M, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	var all []*M
	for k, rows := range s.minutes {
		if len(k) > len(s.Key(c)) && k[:len(s.Key(c))+1] == s.Key(c)+":" {
			all = append(all, rows...)
		}
	}
	return filter(all, a, b, s.MinuteTime), s.Err
}
func (s *Pull[C, D, M, Cov, Scan]) LastDayTime(c C) (string, error) {
	rows, err := s.QueryDay(c, "", "")
	if len(rows) == 0 {
		return "", err
	}
	return s.DayTime(rows[len(rows)-1]), err
}
func (s *Pull[C, D, M, Cov, Scan]) LastMinuteTime(c C, y int) (string, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	rows := s.minutes[s.minKey(c, y)]
	if len(rows) == 0 {
		return "", s.Err
	}
	return s.MinuteTime(rows[len(rows)-1]), s.Err
}
func (s *Pull[C, D, M, Cov, Scan]) SaveDay(c C, from string, rows []*D) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	if len(rows) == 0 {
		return nil
	}
	if s.Err != nil {
		return s.Err
	}
	fresh, err := replace(s.days[s.Key(c)], rows, from, s.DayTime)
	if err != nil {
		return err
	}
	if s.days == nil {
		s.days = map[string][]*D{}
	}
	s.days[s.Key(c)] = fresh
	return nil
}
func (s *Pull[C, D, M, Cov, Scan]) SaveMinute(c C, y int, from string, rows []*M, coverage *Cov) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	if len(rows) == 0 {
		return nil
	}
	if s.Err != nil {
		return s.Err
	}
	k := s.minKey(c, y)
	fresh, err := replace(s.minutes[k], rows, from, s.MinuteTime)
	if err != nil {
		return err
	}
	if s.minutes == nil {
		s.minutes = map[string][]*M{}
	}
	s.minutes[k] = fresh
	if coverage != nil {
		if s.coverage == nil {
			s.coverage = map[string]*Cov{}
		}
		s.coverage[k] = s.Merge(s.coverage[k], coverage)
	}
	return nil
}
func (s *Pull[C, D, M, Cov, Scan]) QueryMinuteCoverage(c C, y int) (*Cov, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	v := s.coverage[s.minKey(c, y)]
	if v == nil {
		return nil, s.Err
	}
	copy := *v
	return &copy, s.Err
}
func (s *Pull[C, D, M, Cov, Scan]) LoadMinuteEmptyScan(c C, y int) (*Scan, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	v := s.scans[s.minKey(c, y)]
	if v == nil {
		return nil, s.Err
	}
	copy := *v
	return &copy, s.Err
}
func (s *Pull[C, D, M, Cov, Scan]) SaveMinuteEmptyScan(c C, y int, v Scan) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	if s.scans == nil {
		s.scans = map[string]*Scan{}
	}
	s.scans[s.minKey(c, y)] = &v
	return s.Err
}
func filter[T any](rows []*T, a, b string, at func(*T) string) []*T {
	out := []*T{}
	for _, v := range rows {
		if (a == "" || at(v) >= a) && (b == "" || at(v) <= b) {
			copy := *v
			out = append(out, &copy)
		}
	}
	sort.Slice(out, func(i, j int) bool { return at(out[i]) < at(out[j]) })
	return out
}
func replace[T any](old, rows []*T, from string, at func(*T) string) ([]*T, error) {
	out := []*T{}
	for _, v := range old {
		if at(v) < from {
			out = append(out, v)
		}
	}
	out = append(out, rows...)
	out = filter(out, "", "", at)
	for i := 1; i < len(out); i++ {
		if at(out[i]) == at(out[i-1]) {
			return nil, errors.New("duplicate source time")
		}
	}
	return out, nil
}
