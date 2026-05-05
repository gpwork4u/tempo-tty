package chart

import (
	"encoding/json"
	"os"
)

type Note struct {
	T    float64 `json:"t"`
	Lane int     `json:"lane"`
}

type Chart struct {
	Audio    string  `json:"audio"`
	Duration float64 `json:"duration"`
	BPM      float64 `json:"bpm"`
	Lanes    int     `json:"lanes"`
	Notes    []Note  `json:"notes"`
}

func Save(path string, c *Chart) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(c)
}

func Load(path string) (*Chart, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Chart
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}
