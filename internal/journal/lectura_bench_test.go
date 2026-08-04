package journal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// journalCon arma un journal con n eventos ya escritos, sin pasar por Append
// (que sería O(n) escrituras y no es lo que se quiere medir).
func journalCon(tb testing.TB, n int) string {
	tb.Helper()
	vault := tb.TempDir()
	dir := filepath.Join(vault, ".cogo", "journal")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		tb.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, "2026-08.jsonl"))
	if err != nil {
		tb.Fatal(err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for i := 1; i <= n; i++ {
		e := Event{
			Seq: uint64(i), NoteID: fmt.Sprintf("nota-%04d", i%500),
			Kind: "CheckDeclared", Emitter: "token:x",
			Payload: json.RawMessage(`{"origen":"bench"}`),
		}
		if err := enc.Encode(e); err != nil {
			tb.Fatal(err)
		}
	}
	return vault
}

// Cuánto cuesta abrir el journal. Open llama a All internamente para ponerse al
// día con la cadena, así que abrirlo NO es barato: es leer y parsear todo.
func BenchmarkOpen(b *testing.B) {
	for _, n := range []int{1_000, 20_000} {
		vault := journalCon(b, n)
		b.Run(fmt.Sprintf("%d-eventos", n), func(b *testing.B) {
			for b.Loop() {
				if _, err := Open(vault); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// Y cuánto cuesta releerlo. Es lo que pasaba en cada llamada a authorize: una
// lectura completa por Open, y otra por All.
func BenchmarkAll(b *testing.B) {
	for _, n := range []int{1_000, 20_000} {
		vault := journalCon(b, n)
		j, err := Open(vault)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(fmt.Sprintf("%d-eventos", n), func(b *testing.B) {
			for b.Loop() {
				if _, err := j.All(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// Escribir ahora toma un cerrojo del sistema y relee la punta del registro. Es
// el precio de que dos procesos no se pisen; conviene tenerlo medido y no
// supuesto.
func BenchmarkAppend(b *testing.B) {
	for _, n := range []int{0, 20_000} {
		vault := journalCon(b, n)
		j, err := Open(vault)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(fmt.Sprintf("sobre-%d-eventos", n), func(b *testing.B) {
			for b.Loop() {
				if _, err := j.Append(Event{NoteID: "n", Kind: "CheckDeclared", Emitter: "bench"}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
