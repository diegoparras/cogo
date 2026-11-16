// Package jev habla con Jev (TypeSafe AI): un modelo que no genera texto, solo
// contesta preguntas acotadas —elegir entre opciones, ubicar en una escala, o
// dar la probabilidad de una condición.
//
// # POR QUÉ ENCAJA EN COGO
//
// COGO tiene una regla de la que sale todo: ningún eje puede subir un color,
// solo bajarlo. Un juez acotado entra por esa misma regla. Sus respuestas son
// techos, votos que solo endurecen, o candidatos que una persona confirma.
// Nunca escribe un color, nunca salta un permiso. Aunque Jev se equivoque, el
// error va para el lado cauto.
//
// Y toda pregunta lleva una opción de abstención: con `unclear`, o sin clave,
// o con el servicio caído, COGO hace exactamente lo que hacía sin Jev.
//
// # EL CONTRATO
//
// POST {model, state, questions} → {model, answers, usage}. Cada respuesta es
// una Choice (etiqueta + distribución), un Score (media ponderada de niveles)
// o un Noul (P de verdadero). No hay prosa que parsear.
package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	urlPorDefecto    = "https://api.typesafe.ai/v1/systemone"
	modeloPorDefecto = "jev-latest"
)

// Pregunta es una de las tres primitivas.
type Pregunta struct {
	Type         string `json:"type"` // choice | score | noul
	Instructions any    `json:"instructions"`
	Criteria     any    `json:"criteria,omitempty"`
}

// Respuesta es lo que vuelve por cada id. Los campos que no corresponden al
// tipo quedan en cero.
type Respuesta struct {
	Type          string             `json:"type"`
	Choice        string             `json:"choice,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Confidence    float64            `json:"confidence,omitempty"`
	Score         float64            `json:"score,omitempty"`
	Noul          float64            `json:"noul,omitempty"`
}

// Cliente es la conexión. Sin clave, no está disponible y todo lo que lo usa
// se comporta como si Jev no existiera.
type Cliente struct {
	URL    string
	Clave  string
	Modelo string
	HTTP   *http.Client
}

// FromEnv lee COGO_JEV_API_KEY (obligatoria), COGO_JEV_MODEL y COGO_JEV_URL.
// La clave va por entorno y no por el registro de parámetros: los secretos no
// se editan desde un panel.
func FromEnv() *Cliente {
	c := &Cliente{
		URL:    strings.TrimSpace(os.Getenv("COGO_JEV_URL")),
		Clave:  strings.TrimSpace(os.Getenv("COGO_JEV_API_KEY")),
		Modelo: strings.TrimSpace(os.Getenv("COGO_JEV_MODEL")),
		HTTP:   &http.Client{Timeout: 15 * time.Second},
	}
	if c.URL == "" {
		c.URL = urlPorDefecto
	}
	if c.Modelo == "" {
		c.Modelo = modeloPorDefecto
	}
	return c
}

// Disponible dice si hay con qué preguntar.
func (c *Cliente) Disponible() bool { return c != nil && c.Clave != "" }

// ErrNoDisponible es lo que devuelve Preguntar sin clave.
var ErrNoDisponible = errors.New("jev: sin clave (COGO_JEV_API_KEY)")

// Preguntar manda un estado y varias preguntas sobre él, y devuelve las
// respuestas por id y el modelo que contestó.
//
// Reintenta una vez ante 429, 529 y 5xx respetando Retry-After (tope 5 s).
// Un 401 o un 422 no se reintentan: son de configuración o del cuerpo, y
// repetirlos no cambia nada.
func (c *Cliente) Preguntar(ctx context.Context, state any, preguntas map[string]Pregunta) (map[string]Respuesta, string, error) {
	if !c.Disponible() {
		return nil, "", ErrNoDisponible
	}
	if len(preguntas) == 0 {
		return map[string]Respuesta{}, c.Modelo, nil
	}
	cuerpo, err := json.Marshal(map[string]any{
		"model": c.Modelo, "state": state, "questions": preguntas,
	})
	if err != nil {
		return nil, "", err
	}
	var ultimo error
	for intento := 0; intento < 2; intento++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(cuerpo))
		if err != nil {
			return nil, "", err
		}
		req.Header.Set("Authorization", "Bearer "+c.Clave)
		req.Header.Set("Content-Type", "application/json")
		res, err := c.HTTP.Do(req)
		if err != nil {
			ultimo = err
			if intento == 0 {
				esperar(ctx, 500*time.Millisecond)
				continue
			}
			break
		}
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
		_ = res.Body.Close()
		switch {
		case res.StatusCode == http.StatusOK:
			var cruda struct {
				Model   string               `json:"model"`
				Answers map[string]Respuesta `json:"answers"`
			}
			if err := json.Unmarshal(b, &cruda); err != nil {
				return nil, "", fmt.Errorf("jev: respuesta ilegible: %w", err)
			}
			if cruda.Answers == nil {
				return nil, "", fmt.Errorf("jev: respuesta sin answers")
			}
			return cruda.Answers, cruda.Model, nil
		case res.StatusCode == http.StatusUnauthorized:
			return nil, "", fmt.Errorf("jev: clave rechazada (401)")
		case res.StatusCode == http.StatusUnprocessableEntity:
			return nil, "", fmt.Errorf("jev: cuerpo inválido (422): %s", recortar(string(b), 300))
		case res.StatusCode == 429 || res.StatusCode == 529 || res.StatusCode >= 500:
			ultimo = fmt.Errorf("jev: %d: %s", res.StatusCode, recortar(string(b), 120))
			if intento == 0 {
				esperar(ctx, espera(res.Header.Get("Retry-After")))
				continue
			}
		default:
			return nil, "", fmt.Errorf("jev: %d: %s", res.StatusCode, recortar(string(b), 200))
		}
	}
	return nil, "", ultimo
}

func espera(retryAfter string) time.Duration {
	if s, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && s > 0 {
		if s > 5 {
			s = 5
		}
		return time.Duration(s) * time.Second
	}
	return 1500 * time.Millisecond
}

func esperar(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

func recortar(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
