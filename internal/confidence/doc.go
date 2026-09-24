// Package confidence contiene la máquina de estados de evidencia de una nota:
// qué situaciones puede atravesar el respaldo de una afirmación y qué eventos
// la mueven entre ellas.
//
// La tabla vive en transitions.yaml y de ahí se genera states_gen.go. No edites
// el archivo generado: editá el YAML y corré `go generate ./internal/confidence`.
//
// Todavía no está conectado al motor vigente. Se construye y se valida aparte a
// propósito, para poder compararlo con el cálculo actual antes de reemplazarlo.
package confidence

//go:generate go run ./gen -in transitions.yaml -out states_gen.go
