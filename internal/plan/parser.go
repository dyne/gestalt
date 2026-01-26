package plan

type Metadata struct {
	Title    string
	Subtitle string
	Date     string
	Keywords string
}

type Heading struct {
	Level    int
	Keyword  string
	Priority string
	Text     string
	Body     string
	Children []Heading
}

type PlanDocument struct {
	Filename  string
	Metadata  Metadata
	Headings  []Heading
	L1Count   int
	L2Count   int
	PriorityA int
	PriorityB int
	PriorityC int
}

type Statistics struct {
	L1Count   int
	L2Count   int
	PriorityA int
	PriorityB int
	PriorityC int
}
