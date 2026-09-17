package mindmap

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"
)

const (
	MaxNodes      = 2000
	MaxLinks      = 2000
	MaxTitleRunes = 120
	MaxTextRunes  = 512
	MaxNoteRunes  = 4000
	MinNodeWidth  = 80
	MaxNodeWidth  = 640
	MinNodeHeight = 32
	MaxNodeHeight = 640
	CanvasExtent  = 100000
	SchemaVersion = 1

	// 2000 nodes carrying the maximum text and note run to roughly 10MB compact,
	// so the document ceiling has to clear what Validate already permits.
	MaxDocumentBytes = 32 << 20
)

var errCycle = errors.New("a node sits inside its own subtree")

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

var paletteAccents = []string{"mauve", "blue", "green", "peach", "pink", "teal", "yellow", "red", "sapphire", "lavender"}

type Node struct {
	ID        string  `json:"id"`
	ParentID  string  `json:"parentId,omitzero"`
	Text      string  `json:"text"`
	Note      string  `json:"note,omitzero"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Width     float64 `json:"width"`
	Height    float64 `json:"height"`
	Accent    string  `json:"accent,omitzero"`
	Collapsed bool    `json:"collapsed,omitzero"`
}

type Link struct {
	ID    string `json:"id"`
	From  string `json:"from"`
	To    string `json:"to"`
	Label string `json:"label,omitzero"`
}

type Map struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	RootID    string    `json:"rootId"`
	Nodes     []Node    `json:"nodes"`
	Links     []Link    `json:"links"`
	Sample    bool      `json:"sample,omitzero"`
	Schema    int       `json:"schema"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Summary struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	NodeCount int       `json:"nodeCount"`
	Sample    bool      `json:"sample,omitzero"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func NewID() string { return strings.ReplaceAll(uuid.NewV7().String(), "-", "")[:24] }

func ValidID(id string) bool { return idPattern.MatchString(id) }

func New(title string) *Map {
	now := time.Now().UTC()
	root := Node{
		ID:     NewID(),
		Text:   strings.TrimSpace(title),
		X:      0,
		Y:      0,
		Width:  200,
		Height: 56,
		Accent: "mauve",
	}
	if root.Text == "" {
		root.Text = "Untitled map"
	}
	return &Map{
		ID:        NewID(),
		Title:     root.Text,
		RootID:    root.ID,
		Nodes:     []Node{root},
		Links:     []Link{},
		Schema:    SchemaVersion,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (m *Map) Summary() Summary {
	return Summary{
		ID:        m.ID,
		Title:     m.Title,
		NodeCount: len(m.Nodes),
		Sample:    m.Sample,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}

func (m *Map) Node(id string) (*Node, bool) {
	for i := range m.Nodes {
		if m.Nodes[i].ID == id {
			return &m.Nodes[i], true
		}
	}
	return nil, false
}

func (m *Map) Normalize() {
	m.Title = clampRunes(strings.TrimSpace(m.Title), MaxTitleRunes)
	if m.Title == "" {
		m.Title = "Untitled map"
	}
	m.Schema = SchemaVersion
	if m.Links == nil {
		m.Links = []Link{}
	}
	for i := range m.Nodes {
		n := &m.Nodes[i]
		n.Text = clampRunes(strings.TrimSpace(n.Text), MaxTextRunes)
		n.Note = clampRunes(strings.TrimSpace(n.Note), MaxNoteRunes)
		n.X = clampCoord(n.X)
		n.Y = clampCoord(n.Y)
		n.Width = clampSize(n.Width, MinNodeWidth, MaxNodeWidth, 200)
		n.Height = clampSize(n.Height, MinNodeHeight, MaxNodeHeight, 56)
		if !slices.Contains(paletteAccents, n.Accent) {
			n.Accent = ""
		}
	}
	for i := range m.Links {
		m.Links[i].Label = clampRunes(strings.TrimSpace(m.Links[i].Label), MaxTitleRunes)
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	m.UpdatedAt = time.Now().UTC()
}

func (m *Map) Validate() error {
	if !ValidID(m.ID) {
		return fmt.Errorf("map id %q is not in the allowed format", m.ID)
	}
	if strings.TrimSpace(m.Title) == "" {
		return errors.New("title is required")
	}
	if utf8.RuneCountInString(m.Title) > MaxTitleRunes {
		return fmt.Errorf("title is longer than %d characters", MaxTitleRunes)
	}
	if len(m.Nodes) == 0 {
		return errors.New("a map needs at least a root node")
	}
	if len(m.Nodes) > MaxNodes {
		return fmt.Errorf("a map holds at most %d nodes, got %d", MaxNodes, len(m.Nodes))
	}
	if len(m.Links) > MaxLinks {
		return fmt.Errorf("a map holds at most %d links, got %d", MaxLinks, len(m.Links))
	}

	seen := make(map[string]struct{}, len(m.Nodes))
	roots := 0
	for _, n := range m.Nodes {
		if !ValidID(n.ID) {
			return fmt.Errorf("node id %q is not in the allowed format", n.ID)
		}
		if _, dup := seen[n.ID]; dup {
			return fmt.Errorf("node id %q appears more than once", n.ID)
		}
		seen[n.ID] = struct{}{}
		if utf8.RuneCountInString(n.Text) > MaxTextRunes {
			return fmt.Errorf("node %q has text longer than %d characters", n.ID, MaxTextRunes)
		}
		if utf8.RuneCountInString(n.Note) > MaxNoteRunes {
			return fmt.Errorf("node %q has a note longer than %d characters", n.ID, MaxNoteRunes)
		}
		if !finite(n.X) || !finite(n.Y) || !finite(n.Width) || !finite(n.Height) {
			return fmt.Errorf("node %q has a non-finite coordinate or size", n.ID)
		}
		if n.ParentID == "" {
			roots++
		}
	}
	if roots != 1 {
		return fmt.Errorf("a map needs exactly one root node, got %d", roots)
	}
	if _, ok := seen[m.RootID]; !ok {
		return fmt.Errorf("root id %q does not name a node", m.RootID)
	}
	if root, _ := m.Node(m.RootID); root.ParentID != "" {
		return errors.New("the root node cannot have a parent")
	}

	for _, n := range m.Nodes {
		if n.ParentID == "" {
			continue
		}
		if _, ok := seen[n.ParentID]; !ok {
			return fmt.Errorf("node %q names a parent %q that does not exist", n.ID, n.ParentID)
		}
		if n.ParentID == n.ID {
			return fmt.Errorf("node %q is its own parent", n.ID)
		}
	}
	if err := m.checkAcyclic(); err != nil {
		return err
	}

	linkIDs := make(map[string]struct{}, len(m.Links))
	for _, l := range m.Links {
		if !ValidID(l.ID) {
			return fmt.Errorf("link id %q is not in the allowed format", l.ID)
		}
		if _, dup := linkIDs[l.ID]; dup {
			return fmt.Errorf("link id %q appears more than once", l.ID)
		}
		linkIDs[l.ID] = struct{}{}
		if l.From == l.To {
			return fmt.Errorf("link %q joins a node to itself", l.ID)
		}
		if _, ok := seen[l.From]; !ok {
			return fmt.Errorf("link %q starts at a node %q that does not exist", l.ID, l.From)
		}
		if _, ok := seen[l.To]; !ok {
			return fmt.Errorf("link %q ends at a node %q that does not exist", l.ID, l.To)
		}
	}
	return nil
}

func (m *Map) checkAcyclic() error {
	parent := make(map[string]string, len(m.Nodes))
	for _, n := range m.Nodes {
		parent[n.ID] = n.ParentID
	}
	const (
		unvisited = 0
		active    = 1
		done      = 2
	)
	state := make(map[string]int, len(m.Nodes))
	for _, n := range m.Nodes {
		if state[n.ID] != unvisited {
			continue
		}
		var path []string
		cur := n.ID
		for cur != "" && state[cur] == unvisited {
			state[cur] = active
			path = append(path, cur)
			cur = parent[cur]
		}
		if cur != "" && state[cur] == active {
			return fmt.Errorf("%w: node %q", errCycle, cur)
		}
		for _, id := range path {
			state[id] = done
		}
	}
	return nil
}

func clampRunes(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	return string([]rune(s)[:limit])
}

func clampCoord(v float64) float64 {
	if !finite(v) {
		return 0
	}
	return math.Round(min(max(v, -CanvasExtent), CanvasExtent)*100) / 100
}

func clampSize(v, lo, hi, fallback float64) float64 {
	if !finite(v) || v <= 0 {
		return fallback
	}
	return math.Round(min(max(v, lo), hi)*100) / 100
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
