package main

import (
	"strings"
	"testing"
)

// mcpBlanco es lo que convierte la auditoría de "hay alguien conectado" en "hay
// alguien trabajando ACÁ". Sin el blanco, presencia no puede filtrar por
// proyecto y el aviso saldría en todos lados.
func TestMcpBlancoSacaSobreQueSeLlamo(t *testing.T) {
	casos := []struct {
		nombre         string
		body           string
		nota, proyecto string
	}{
		{"capture con id", `{"method":"tools/call","params":{"name":"capture",
			"arguments":{"id":"db-migracion","project":"cogo"}}}`, "db-migracion", "cogo"},
		{"verify usa note", `{"method":"tools/call","params":{"name":"verify",
			"arguments":{"note":"db-migracion"}}}`, "db-migracion", ""},
		{"lease usa name", `{"method":"tools/call","params":{"name":"lease",
			"arguments":{"name":"migrar-db"}}}`, "migrar-db", ""},
		{"pack solo trae proyecto", `{"method":"tools/call","params":{"name":"pack",
			"arguments":{"query":"como migro","project":"cogo"}}}`, "", "cogo"},
		{"basura no rompe", `no es json`, "", ""},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			n, p := mcpBlanco([]byte(c.body))
			if n != c.nota || p != c.proyecto {
				t.Errorf("esperaba (%q,%q), vino (%q,%q)", c.nota, c.proyecto, n, p)
			}
		})
	}
}

// Lo que NO tiene que guardar: el texto libre. Un `query` o un `body` pueden
// decir cosas que no corresponde dejar en un log que después se lee entero para
// armar un aviso.
func TestMcpBlancoNoGuardaTextoLibre(t *testing.T) {
	body := `{"method":"tools/call","params":{"name":"capture","arguments":{
		"body":"la clave de produccion es hunter2","query":"secreto","project":"cogo"}}}`
	n, p := mcpBlanco([]byte(body))
	if n != "" {
		t.Errorf("no debería sacar nada de un capture sin id, vino %q", n)
	}
	if p != "cogo" {
		t.Errorf("el proyecto sí, vino %q", p)
	}
	if strings.Contains(n+p, "hunter2") {
		t.Fatal("el cuerpo de una nota no puede terminar en la auditoría")
	}
}
