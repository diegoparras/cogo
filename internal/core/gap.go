package core

import "sort"

// TipoBrecha es el tipo de nota que modela lo que el proyecto NO sabe.
const TipoBrecha = "gap"

// EsBrecha dice si una nota es una pregunta abierta en vez de una afirmación.
func EsBrecha(n *Note) bool { return n != nil && n.Type == TipoBrecha }

// Brechas devuelve las preguntas abiertas del vault, ordenadas por cuántas
// decisiones traban y, a igualdad, por cuánto cuesta resolverlas.
//
// El orden es una heurística del valor de la información: conviene contestar
// primero lo que destraba más cosas, y entre dos que destraban lo mismo, lo que
// sale más barato averiguar. Alcanza con contar; no hace falta un modelo.
func Brechas(vault map[string]*Note) []*Note {
	var out []*Note
	for _, n := range vault {
		if EsBrecha(n) {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		bi, bj := len(out[i].Blocks), len(out[j].Blocks)
		if bi != bj {
			return bi > bj
		}
		ci, cj := ordenCosto(out[i].CostToResolve), ordenCosto(out[j].CostToResolve)
		if ci != cj {
			return ci < cj
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func ordenCosto(c string) int {
	switch c {
	case "bajo", "low":
		return 0
	case "medio", "medium":
		return 1
	case "alto", "high":
		return 2
	}
	return 1 // sin declarar: se asume intermedio
}

// BloqueadasPor invierte la relación: dada una nota, qué preguntas abiertas la
// están trabando. Es lo que permite decirle a alguien que mira una decisión
// "esto está esperando que averigües X".
func BloqueadasPor(vault map[string]*Note, noteID string) []*Note {
	var out []*Note
	for _, n := range Brechas(vault) {
		for _, b := range n.Blocks {
			if b == noteID {
				out = append(out, n)
				break
			}
		}
	}
	return out
}
