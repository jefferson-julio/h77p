package script

type PreContext struct {
	Request *ScriptRequest
	Env     map[string]string
	Logs    []string
}

type PostContext struct {
	Request  *ScriptRequest
	Response *ScriptResponse
	Env      map[string]string
	Logs     []string
}

type ScriptRequest struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    string
}

type ScriptResponse struct {
	Status     int
	StatusText string
	Headers    map[string]string
	Body       string
	Duration   int64
}

type TestResult struct {
	Name   string
	Passed bool
	Error  string
}
