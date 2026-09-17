package mindmap

import (
	"math"
	"testing"
)

func mapOf(nodes []Node, links []Link) *Map {
	return &Map{
		ID:     "aaaaaaaaaaaaaaaaaaaaaaaa",
		Title:  "Test map",
		RootID: nodes[0].ID,
		Nodes:  nodes,
		Links:  links,
		Schema: SchemaVersion,
	}
}

func node(id, parent string) Node {
	return Node{ID: id, ParentID: parent, Text: id, Width: 180, Height: 48}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		build   func() *Map
		wantErr bool
	}{
		{
			name:    "root only",
			build:   func() *Map { return mapOf([]Node{node("root", "")}, nil) },
			wantErr: false,
		},
		{
			name: "duplicate node id",
			build: func() *Map {
				return mapOf([]Node{node("root", ""), node("dup", "root"), node("dup", "root")}, nil)
			},
			wantErr: true,
		},
		{
			name: "parent that does not exist",
			build: func() *Map {
				return mapOf([]Node{node("root", ""), node("orphan", "ghost")}, nil)
			},
			wantErr: true,
		},
		{
			name: "two nodes parenting each other",
			build: func() *Map {
				return mapOf([]Node{node("root", ""), node("a", "b"), node("b", "a")}, nil)
			},
			wantErr: true,
		},
		{
			name: "node that is its own parent",
			build: func() *Map {
				return mapOf([]Node{node("root", ""), node("self", "self")}, nil)
			},
			wantErr: true,
		},
		{
			name: "second root",
			build: func() *Map {
				return mapOf([]Node{node("root", ""), node("other", "")}, nil)
			},
			wantErr: true,
		},
		{
			name: "root id naming no node",
			build: func() *Map {
				m := mapOf([]Node{node("root", "")}, nil)
				m.RootID = "missing"
				return m
			},
			wantErr: true,
		},
		{
			name: "node id that would escape the data directory",
			build: func() *Map {
				return mapOf([]Node{node("root", ""), node("../escape", "root")}, nil)
			},
			wantErr: true,
		},
		{
			name: "link to a node that does not exist",
			build: func() *Map {
				return mapOf([]Node{node("root", ""), node("a", "root")},
					[]Link{{ID: "l1", From: "a", To: "ghost"}})
			},
			wantErr: true,
		},
		{
			name: "link joining a node to itself",
			build: func() *Map {
				return mapOf([]Node{node("root", ""), node("a", "root")},
					[]Link{{ID: "l1", From: "a", To: "a"}})
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.build().Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNormalizeRepairsGeometryValidateWouldReject(t *testing.T) {
	m := mapOf([]Node{node("root", "")}, nil)
	m.Title = "   "
	m.Nodes[0].X = math.Inf(1)
	m.Nodes[0].Y = math.NaN()
	m.Nodes[0].Width = 0
	m.Nodes[0].Height = -40
	m.Nodes[0].Accent = "not-a-palette-colour"
	m.Nodes[0].Text = "  padded  "

	if err := m.Validate(); err == nil {
		t.Fatal("Validate() accepted the broken map, so this test proves nothing")
	}
	m.Normalize()
	if err := m.Validate(); err != nil {
		t.Fatalf("Normalize left the map invalid: %v", err)
	}

	n := m.Nodes[0]
	if n.X != 0 || n.Y != 0 {
		t.Errorf("coordinates = %v,%v, want both zeroed", n.X, n.Y)
	}
	if n.Width < MinNodeWidth || n.Height < MinNodeHeight {
		t.Errorf("size = %vx%v, want both at or above the minimum", n.Width, n.Height)
	}
	if n.Accent != "" {
		t.Errorf("accent = %q, want an unknown accent dropped", n.Accent)
	}
	if n.Text != "padded" {
		t.Errorf("text = %q, want it trimmed", n.Text)
	}
}

func TestLayout(t *testing.T) {
	build := func() *Map {
		return mapOf([]Node{
			node("root", ""),
			node("a", "root"), node("a1", "a"), node("a2", "a"),
			node("b", "root"), node("b1", "b"),
			node("loose", "root"),
		}, nil)
	}

	overlapping := func(m *Map, first, second string) bool {
		x, _ := m.Node(first)
		y, _ := m.Node(second)
		return x.Y < y.Y+y.Height && y.Y < x.Y+x.Height
	}

	t.Run("siblings are stacked without overlapping", func(t *testing.T) {
		m := build()
		Layout(m)
		if overlapping(m, "a", "b") {
			t.Error("two top-level branches overlap vertically")
		}
		if overlapping(m, "a1", "a2") {
			t.Error("two children of the same node overlap vertically")
		}
		root, _ := m.Node("root")
		a, _ := m.Node("a")
		if a.X <= root.X+root.Width {
			t.Errorf("child X = %v, want it right of the parent edge at %v", a.X, root.X+root.Width)
		}
	})

	t.Run("a collapsed branch reserves no room for its children", func(t *testing.T) {
		open := build()
		Layout(open)
		openA, _ := open.Node("a")
		openB, _ := open.Node("b")
		spread := openB.Y - openA.Y

		closed := build()
		toClose, _ := closed.Node("a")
		toClose.Collapsed = true
		Layout(closed)
		closedA, _ := closed.Node("a")
		closedB, _ := closed.Node("b")

		if closedB.Y-closedA.Y >= spread {
			t.Errorf("collapsing a branch did not tighten the layout: %v, want under %v", closedB.Y-closedA.Y, spread)
		}
	})

	t.Run("a node the root cannot reach is parked beside the tree", func(t *testing.T) {
		m := build()
		orphan, _ := m.Node("loose")
		orphan.ParentID = "gone"
		orphan.X = 5000
		orphan.Y = 5000
		Layout(m)

		orphan, _ = m.Node("loose")
		root, _ := m.Node("root")
		if orphan.X >= root.X {
			t.Errorf("unreachable node X = %v, want it parked left of the root at %v", orphan.X, root.X)
		}
	})
}
