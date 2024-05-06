package asterix

import "encoding/json"

type GraphQlQuery struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

func (q GraphQlQuery) ToJSON() ([]byte, error) {
	return json.Marshal(q)
}
