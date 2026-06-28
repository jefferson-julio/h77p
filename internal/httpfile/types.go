package httpfile

// FileItem records the document order of top-level blocks (requests and groups).
type FileItem struct {
	IsGroup bool
	Index   int // index into File.Groups (IsGroup=true) or File.Requests (IsGroup=false)
}

type File struct {
	Path      string
	Variables []Variable
	Groups    []Group
	Requests  []Request
	Items     []FileItem // document order of requests and groups
}

// Group is a named collection of requests loaded from an @import directive.
type Group struct {
	Name   string // from the preceding ### Name separator
	Source string // relative path as written in the @import line
	File   *File  // fully parsed imported file; nil if the file could not be loaded
}

type Variable struct {
	Name  string
	Value string
}

type Request struct {
	Name      string
	FilePath  string // absolute path of the .http file that defines this request
	Variables []Variable
	Method    string
	URL       string
	Version   string // optional HTTP version override, e.g. "HTTP/1.1", "HTTP/2", "HTTP/3"
	Headers   []Header
	Body      string
	PreScript  string
	JQFilters  []string // @jq lines — applied to response body in order
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
