package ipc

// Command is a message sent from the client (e.g. a Neovim plugin) to h77p.
// The Cmd field is the discriminant.
type Command struct {
	Cmd     string `json:"cmd"`
	File    string `json:"file,omitempty"`
	Request string `json:"request,omitempty"`
	Key     string `json:"key,omitempty"`
	Value   string `json:"value,omitempty"`
	Content string `json:"content,omitempty"` // used by save_edit
}

// Event is a message sent from h77p to the client.
// The Event field is the discriminant.
type Event struct {
	Event      string            `json:"event"`
	File       string            `json:"file,omitempty"`
	Request    string            `json:"request,omitempty"`
	Line       int               `json:"line,omitempty"`
	Key        string            `json:"key,omitempty"`
	Value      string            `json:"value,omitempty"`
	Status     int               `json:"status,omitempty"`
	DurationMs int64             `json:"duration_ms,omitempty"`
	Passed     bool              `json:"passed,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
}
