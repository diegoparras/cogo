package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/diegoparras/cogo/internal/core"
)

// /healthz decía "ok" pase lo que pase: un vault ilegible o un registro
// inaccesible seguían reportando salud. Un healthcheck que no puede fallar no
// es un healthcheck.
//
// Chequea lo mínimo que hace a COGO servible: que el vault se pueda recorrer
// (una lectura barata, por mtime) y que el registro de eventos abra. No chequea
// la cadena entera —eso es O(n) y va en la sala de guerra y en la lista de
// notas— porque un healthcheck corre cada treinta segundos.
func salud(dir string) http.HandlerFunc {
	cache := core.NewVaultCache(dir)
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := os.Stat(dir); err != nil {
			http.Error(w, "vault: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
		if _, err := cache.Load(); err != nil {
			http.Error(w, "vault: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
		if _, err := journalDe(dir); err != nil {
			http.Error(w, "registro: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}
}

// cmdHealth es lo que corre el HEALTHCHECK de la imagen: en scratch no hay
// curl ni shell, así que el binario se prueba a sí mismo.
func cmdHealth(args []string) error {
	fs := flag.NewFlagSet("health", flag.ExitOnError)
	url := fs.String("url", "http://127.0.0.1:8080/healthz", "qué probar")
	_ = fs.Parse(args)
	cli := &http.Client{Timeout: 4 * time.Second}
	res, err := cli.Get(*url)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	cuerpo, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %d %s", *url, res.StatusCode, string(cuerpo))
	}
	fmt.Println("ok")
	return nil
}
