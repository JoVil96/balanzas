package domain

import "time"

type Reading struct {
	Weight    float64   `json:"weight"`
	Unit      string    `json:"unit"`
	Stable    bool      `json:"stable"`
	Raw       string    `json:"raw"`
	Timestamp time.Time `json:"timestamp"`
}
