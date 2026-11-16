package agentsmd

import "strings"

// Skill devuelve el SKILL.md del plugin de Claude Code: el mismo protocolo que
// el AGENTS.md, más lo que en el plugin hace el harness por su cuenta. La
// conexión no va: la arma el plugin con la dirección y el token que la persona
// cargó al instalarlo.
func Skill() string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("name: cogo\n")
	b.WriteString("description: Usar en cualquier proyecto conectado a COGO. Antes de responder o cambiar algo, pedir contexto con pack; ")
	b.WriteString("antes de una acción costosa o irreversible, authorize; después de compactar, recall; y capturar lo verificado sin escribir el color.\n")
	b.WriteString("---\n\n")
	b.WriteString("# COGO: la memoria con semáforo\n\n")
	b.WriteString("Este proyecto usa **COGO** como memoria: notas con un **color de confianza computado** ")
	b.WriteString("(verde/amarillo/rojo). El color ya viene calculado por COGO — no lo decidís vos, lo obedecés.\n\n")

	protocolo(&b)

	b.WriteString("## Antes de actuar: `authorize`\n\n")
	b.WriteString("Antes de algo que cambia cosas afuera de tu respuesta y es costoso o irreversible —borrar, migrar, desplegar, publicar— ")
	b.WriteString("preguntale a COGO con `authorize` si lo que sabés alcanza. Este plugin trae hooks que lo hacen solos antes de cada comando y cada edición, ")
	b.WriteString("pero si vas a hacer algo que no pasa por esas herramientas, llamalo vos, con las notas en las que te apoyás o con `find_support: true`.\n\n")
	b.WriteString("- Si contesta `NOT AUTHORIZED`, no lo rodees: contale a la persona qué falta, con el número de recibo, y dejá que decida.\n")
	b.WriteString("- Si contesta `AUTHORIZED BY THE PERSON`, COGO no lo autorizaba y la persona lo aprobó: seguí, y decí que fue decisión suya.\n")
	b.WriteString("- Si el diálogo de permiso te pregunta algo de parte de COGO, es la persona la que decide, no vos.\n\n")
	b.WriteString("## Después de compactar: `recall`\n\n")
	b.WriteString("Cuando el contexto se compacta se pierden las reglas. El plugin llama a `recall` solo después de compactar, retomar o limpiar la sesión; ")
	b.WriteString("si no lo ves en el contexto, llamalo vos: trae el mandato, las líneas rojas y las restricciones verdes.\n")
	return b.String()
}
