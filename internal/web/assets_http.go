package web

import (
	"fmt"
	"hash/fnv"
	"io/fs"
	"net/http"
)

// El caché de los assets del visor.
//
// # EL PROBLEMA
//
// Los archivos del visor salen del embed.FS del binario, y un embed.FS no tiene
// fecha de modificación: los sirve sin ETag ni Last-Modified. El navegador los
// cachea y no tiene con qué revalidarlos, así que se queda con la copia vieja
// para siempre.
//
// La solución anterior era un `?v=3` en el HTML con un comentario que decía
// "subí el número CADA VEZ que toques app.js". Eso no es una solución: es una
// tarea que hay que recordar, y las tareas que hay que recordar se olvidan. Se
// olvidó tres veces en un solo día de trabajo, y el síntoma es de los peores —
// el visor sigue mostrando la versión anterior, sin ningún error, hasta que a
// alguien se le ocurre un Ctrl+F5.
//
// # LA SOLUCIÓN
//
// Un ETag derivado del CONTENIDO, calculado una vez al arrancar. El navegador
// pregunta "¿sigue siendo este?" y recibe un 304 de dos bytes mientras no
// cambie; el día que cambia, recibe el archivo nuevo. Sin números que subir y
// sin nada que recordar.
//
// El hash es FNV, no criptográfico: acá solo hace falta que dos contenidos
// distintos den etiquetas distintas, no resistir a un adversario.
func servidorDeAssets(sub fs.FS) http.Handler {
	etags := map[string]string{}
	_ = fs.WalkDir(sub, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, err := fs.ReadFile(sub, p)
		if err != nil {
			return nil
		}
		h := fnv.New64a()
		_, _ = h.Write(b)
		etags["/"+p] = fmt.Sprintf(`"%016x"`, h.Sum64())
		return nil
	})

	archivos := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ruta := r.URL.Path
		if ruta == "/" {
			ruta = "/index.html"
		}
		if et, ok := etags[ruta]; ok {
			// no-cache NO significa "no lo guardes": significa "guardalo, pero
			// preguntá antes de usarlo". Es exactamente lo que hace falta.
			w.Header().Set("ETag", et)
			w.Header().Set("Cache-Control", "no-cache")
			if r.Header.Get("If-None-Match") == et {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}
		archivos.ServeHTTP(w, r)
	})
}
