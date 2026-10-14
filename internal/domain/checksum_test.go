package domain

import "testing"

// Vectores calculados con `sha256sum`, fuera del código Go: la fuente de verdad es
// independiente de la función que se prueba.
func TestNewChecksum(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"vacío", "", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"texto", "texto", "f46e3b17a6b639e5efddc3d1498ad73491599c22220f7ef6e14772f4ee25b913"},
		{"frase", "texto plano del documento", "03ebb33a77b4785b9c3962d1b1da3ae0c3358c121d118f6c71aeb72dd9cdc456"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NewChecksum(tc.text); string(got) != tc.want {
				t.Fatalf("NewChecksum(%q) = %q, quería %q", tc.text, got, tc.want)
			}
		})
	}
}
