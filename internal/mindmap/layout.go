package mindmap

import "slices"

const (
	layoutGapX = 72.0
	layoutGapY = 20.0
)

func Layout(m *Map) {
	if len(m.Nodes) == 0 {
		return
	}
	byID := make(map[string]*Node, len(m.Nodes))
	for i := range m.Nodes {
		byID[m.Nodes[i].ID] = &m.Nodes[i]
	}
	children := make(map[string][]string, len(m.Nodes))
	for _, n := range m.Nodes {
		if n.ParentID != "" && byID[n.ParentID] != nil {
			children[n.ParentID] = append(children[n.ParentID], n.ID)
		}
	}
	for id, order := range children {
		slices.SortStableFunc(order, func(a, b string) int {
			switch {
			case byID[a].Y < byID[b].Y:
				return -1
			case byID[a].Y > byID[b].Y:
				return 1
			default:
				return 0
			}
		})
		children[id] = order
	}

	root, ok := byID[m.RootID]
	if !ok {
		root = &m.Nodes[0]
		m.RootID = root.ID
	}

	spans := make(map[string]float64, len(m.Nodes))
	var span func(id string) float64
	span = func(id string) float64 {
		if v, done := spans[id]; done {
			return v
		}
		spans[id] = byID[id].Height
		n := byID[id]
		kids := children[id]
		if n.Collapsed || len(kids) == 0 {
			return spans[id]
		}
		total := 0.0
		for i, k := range kids {
			if i > 0 {
				total += layoutGapY
			}
			total += span(k)
		}
		spans[id] = max(total, n.Height)
		return spans[id]
	}

	reachable := make(map[string]bool, len(m.Nodes))
	var place func(id string, left, top float64)
	place = func(id string, left, top float64) {
		if reachable[id] {
			return
		}
		reachable[id] = true
		n := byID[id]
		n.X = clampCoord(left)
		n.Y = clampCoord(top + (span(id)-n.Height)/2)
		childTop := top
		childLeft := left + n.Width + layoutGapX
		for _, k := range children[id] {
			place(k, childLeft, childTop)
			childTop += span(k) + layoutGapY
		}
	}
	place(root.ID, 0, 0)

	offset := 0.0
	for i := range m.Nodes {
		n := &m.Nodes[i]
		if reachable[n.ID] {
			continue
		}
		n.X = clampCoord(-n.Width - layoutGapX*3)
		n.Y = clampCoord(offset)
		offset += n.Height + layoutGapY
	}
}
