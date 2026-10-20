package asterix

type GraphQlQuery struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

type graphQlError struct {
	Message string `json:"message"`
}
