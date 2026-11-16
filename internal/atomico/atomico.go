// Package atomico escribe archivos de estado de forma que un corte a mitad de
// la escritura no deje el archivo por la mitad.
//
// # POR QUÉ
//
// Todo el estado de COGO son archivos chicos que se reescriben enteros: la
// nota, parametros.json, tokens.json, leases.json, contradictions.json, uso.json.
// os.WriteFile trunca y después escribe; si se corta la luz —o se mata el
// contenedor— entre las dos cosas, queda un archivo vacío o truncado. Para una
// nota es una nota ilegible; para tokens.json es que nadie puede entrar.
//
// # CÓMO
//
// Se escribe a un archivo temporal EN EL MISMO DIRECTORIO (rename solo es
// atómico dentro de un sistema de archivos), se hace fsync, y se renombra
// encima del destino. El rename es atómico en POSIX y en Windows (MoveFileEx
// con REPLACE_EXISTING): en cualquier instante hay o el archivo viejo entero o
// el nuevo entero, nunca una mezcla.
package atomico

import (
	"os"
	"path/filepath"
)

// Escribir reemplaza el contenido de `path` de forma atómica.
func Escribir(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	nombre := tmp.Name()
	limpiar := func() { _ = tmp.Close(); _ = os.Remove(nombre) }
	if _, err := tmp.Write(data); err != nil {
		limpiar()
		return err
	}
	if err := tmp.Sync(); err != nil {
		limpiar()
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(nombre)
		return err
	}
	if err := os.Chmod(nombre, perm); err != nil {
		_ = os.Remove(nombre)
		return err
	}
	if err := os.Rename(nombre, path); err != nil {
		_ = os.Remove(nombre)
		return err
	}
	return nil
}
