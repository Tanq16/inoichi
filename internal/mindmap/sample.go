package mindmap

import "time"

type sampleNode struct {
	text     string
	accent   string
	note     string
	children []sampleNode
}

var sampleTree = sampleNode{
	text:   "Sample map (delete me)",
	accent: "mauve",
	note:   "This map ships with Inoichi as example content. Delete it from the map list; nothing else depends on it.",
	children: []sampleNode{
		{
			text:   "How to move around",
			accent: "blue",
			children: []sampleNode{
				{text: "Drag the background to pan"},
				{text: "Scroll to zoom, Ctrl+0 to reset"},
				{text: "Arrow keys walk the tree"},
			},
		},
		{
			text:   "How to build a map",
			accent: "green",
			children: []sampleNode{
				{text: "Tab adds a child to the selected node"},
				{text: "Enter adds a sibling"},
				{text: "F2 renames, Delete removes a branch"},
				{text: "Shift+drag a node onto another to reparent it"},
			},
		},
		{
			text:   "Where your data lives",
			accent: "peach",
			children: []sampleNode{
				{text: "One JSON file per map on this machine"},
				{text: "Nothing leaves the process, ever"},
				{text: "Export JSON, SVG or PNG any time"},
			},
		},
		{
			text:   "What this is not",
			accent: "red",
			children: []sampleNode{
				{text: "No sharing, no accounts, no sync"},
				{text: "No .xmind import or export"},
				{text: "No template gallery"},
			},
		},
	},
}

func Sample() *Map {
	now := time.Now().UTC()
	m := &Map{
		ID:        NewID(),
		Title:     sampleTree.text,
		Links:     []Link{},
		Sample:    true,
		Schema:    SchemaVersion,
		CreatedAt: now,
		UpdatedAt: now,
	}
	root := buildSample(m, sampleTree, "")
	m.RootID = root
	Layout(m)
	return m
}

func buildSample(m *Map, s sampleNode, parentID string) string {
	width, height := 200.0, 56.0
	if parentID != "" {
		width, height = 220.0, 48.0
	}
	n := Node{
		ID:       NewID(),
		ParentID: parentID,
		Text:     s.text,
		Note:     s.note,
		Accent:   s.accent,
		Width:    width,
		Height:   height,
	}
	m.Nodes = append(m.Nodes, n)
	for _, c := range s.children {
		buildSample(m, c, n.ID)
	}
	return n.ID
}
