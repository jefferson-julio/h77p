package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jefferson-julio/h77p/internal/httpfile"
)

var selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))

type Model struct {
	file     *httpfile.File
	selected int
}

func New(file *httpfile.File) Model {
	return Model{file: file}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return m, nil
}

func (m Model) View() string {
	if m.file == nil {
		return ""
	}
	var out string
	for i, r := range m.file.Requests {
		line := r.Method + " " + r.Name
		if i == m.selected {
			line = selectedStyle.Render(line)
		}
		out += line + "\n"
	}
	return out
}

func Start(file *httpfile.File) error {
	p := tea.NewProgram(New(file))
	_, err := p.Run()
	return err
}
