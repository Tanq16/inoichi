package mindmap

import "testing"

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
		nodes   []Node
		links   []Link
		rootID  string
		wantErr bool
	}{
		{"root only", []Node{node("root", "")}, nil, "", false},
		{"node id that would escape the data directory", []Node{node("root", ""), node("../escape", "root")}, nil, "", true},
		{"duplicate node id", []Node{node("root", ""), node("dup", "root"), node("dup", "root")}, nil, "", true},
		{"parent that does not exist", []Node{node("root", ""), node("orphan", "ghost")}, nil, "", true},
		{"two nodes parenting each other", []Node{node("root", ""), node("a", "b"), node("b", "a")}, nil, "", true},
		{"second root", []Node{node("root", ""), node("other", "")}, nil, "", true},
		{"root id naming a node with a parent", []Node{node("root", ""), node("a", "root")}, nil, "a", true},
		{"link to a node that does not exist", []Node{node("root", ""), node("a", "root")}, []Link{{ID: "l1", From: "a", To: "ghost"}}, "", true},
		{"link joining a node to itself", []Node{node("root", ""), node("a", "root")}, []Link{{ID: "l1", From: "a", To: "a"}}, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := mapOf(tt.nodes, tt.links)
			if tt.rootID != "" {
				m.RootID = tt.rootID
			}
			if err := m.Validate(); (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
