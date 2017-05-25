package service

type dag struct {
	nodes map[Type]*node
}

type node struct {
	t            Type
	started      bool
	opts         ConfigOpts
	instance     Service
	indegree     int
	pendingVisit int
	children     []*node
}

func newDag() *dag {
	return &dag{nodes: make(map[Type]*node)}
}

func (this *dag) AddVertex(t Type, instance Service, opts ConfigOpts) *node {
	n := &node{t: t, instance: instance, opts: opts}
	this.nodes[t] = n
	return n
}

func (this *dag) AddEdge(from, to Type) {
	fromNode := this.nodes[from]
	toNode := this.nodes[to]
	fromNode.children = append(fromNode.children, toNode)
	toNode.indegree++
}

func (this *dag) Lookup(t Type) *node {
	return this.nodes[t]
}
func (this *dag) Flatten() []*node {
	candidate := make([]*node, 0, len(this.nodes))
	result := make([]*node, 0, len(this.nodes))
	for _, n := range this.nodes {
		if n.indegree == 0 {
			candidate = append(candidate, n)
		} else {
			n.pendingVisit = n.indegree
		}
	}
	var n *node
	for len(candidate) > 0 {
		n, candidate = candidate[0], candidate[1:]
		result = append(result, n)
		for _, c := range n.children {
			c.pendingVisit--
			if c.pendingVisit == 0 {
				candidate = append(candidate, c)
			}
		}
	}
	return result
}
