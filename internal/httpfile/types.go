package httpfile

type File struct {
	Path      string
	Variables []Variable
	Imports   []string
	Requests  []Request
}

type Variable struct {
	Name  string
	Value string
}

type Request struct {
	Name       string
	Variables  []Variable
	Method     string
	URL        string
	Headers    []Header
	Body       string
	PreScript  string
	PostScript string
	Example    *Example
}

type Header struct {
	Name  string
	Value string
}

type Example struct {
	Status  string
	Headers []Header
	Body    string
}
