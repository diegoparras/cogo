package ancla

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Dónde se publica un sello.
//
// COGO no elige por vos, y no trae un destino "por default que anda solo": eso
// sería mandar la cabeza de tu registro a un servidor de un tercero sin que lo
// hayas pedido, y COGO es offline salvo que se le diga otra cosa.
//
// Los dos destinos cubren las dos formas honestas de hacerlo:
//
//	manual  COGO calcula el sello y te lo da. Vos lo publicás donde quieras
//	        —un commit firmado, un mail, un mensaje— y anotás dónde quedó.
//	        Cero red, y sirve: lo que importa es que exista una copia con
//	        fecha en un lugar que vos no puedas reescribir solo.
//
//	https   COGO lo manda a una URL que le des. Un log de transparencia, un
//	        endpoint de un socio, un bucket con object-lock. El sello guarda
//	        lo que haya contestado.
//
// Falta OpenTimestamps, que sería el mejor destino público —ancla a Bitcoin,
// gratis, sin billetera— pero su formato de prueba tiene una serialización
// propia, y un archivo .ots mal armado es peor que no tener ninguno: parece una
// prueba y no verifica. Va cuando esté hecho bien, no antes.
const (
	DestinoManual = "manual"
	DestinoHTTPS  = "https"
)

// Destinos son los válidos, para el registro de parámetros.
var Destinos = []string{DestinoManual, DestinoHTTPS}

// Publicar manda la cabeza al destino y devuelve el recibo.
//
// El destino manual no publica nada: devuelve un recibo vacío, y es
// responsabilidad de quien lo corre publicarlo y anotar dónde. Que no sea
// automático es la razón por la que sirve — un sello que COGO se manda a sí
// mismo no prueba nada contra COGO.
func Publicar(destino, url string, seq uint64, cabeza string) (recibo string, err error) {
	switch destino {
	case DestinoManual, "":
		return "", nil

	case DestinoHTTPS:
		if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
			return "", fmt.Errorf("ancla: el destino https necesita una URL (hay %q)", url)
		}
		cuerpo, _ := json.Marshal(map[string]any{
			"seq": seq, "cabeza": cabeza, "cuando": time.Now().UTC().Format(time.RFC3339),
		})
		cli := &http.Client{Timeout: 15 * time.Second}
		resp, err := cli.Post(url, "application/json", bytes.NewReader(cuerpo))
		if err != nil {
			return "", fmt.Errorf("ancla: no se pudo publicar en %s: %w", url, err)
		}
		defer resp.Body.Close()
		// Se guarda lo que haya contestado, acotado: es el recibo con el que
		// después se va a buscar. Un cuerpo enorme no sirve de recibo.
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if resp.StatusCode >= 300 {
			return "", fmt.Errorf("ancla: %s contestó %d: %s", url, resp.StatusCode, strings.TrimSpace(string(b)))
		}
		return strings.TrimSpace(string(b)), nil
	}
	return "", fmt.Errorf("ancla: destino desconocido %q", destino)
}

// ComoPublicarloAMano es lo que se le muestra a alguien que sella sin destino
// automático. Un sello que no se publica no sirve, así que el texto tiene que
// decir el paso siguiente, no felicitar por el hash.
func ComoPublicarloAMano(seq uint64, cabeza string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "COGO · sello del registro\n")
	fmt.Fprintf(&b, "  evento:  %d\n", seq)
	fmt.Fprintf(&b, "  cabeza:  %s\n", cabeza)
	fmt.Fprintf(&b, "  cuando:  %s\n", time.Now().UTC().Format(time.RFC3339))
	b.WriteString("\nPublicá ESTAS TRES LÍNEAS en algún lugar que no puedas reescribir solo:\n")
	b.WriteString("un commit firmado, un mail a alguien, un mensaje con fecha.\n")
	b.WriteString("Después anotá dónde quedó — sin eso, el sello es un hash guardado\n")
	b.WriteString("al lado del registro que resume, y no prueba nada contra vos.\n")
	return b.String()
}
