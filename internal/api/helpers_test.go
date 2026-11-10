package api

import "encoding/json"

// decodeJSON decodifica el cuerpo de una respuesta en los tests.
func decodeJSON(data []byte, target any) error {
	return json.Unmarshal(data, target)
}
