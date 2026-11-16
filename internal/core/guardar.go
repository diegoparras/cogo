package core

// EscribirConColor escribe la nota con el color que le corresponde DESPUÉS de
// escribirla, y devuelve ese veredicto.
//
// Hace falta porque el color puede depender de la escritura misma. Con el motor
// de eventos instalado, el color se pliega desde el registro, y lo que mueve el
// eje del check —una verificación declarada, por ejemplo— entra al registro por
// el hook de WriteNoteFile. Evaluar antes de escribir le preguntaba al motor por
// la nota de antes: verify contestaba amarillo sobre una nota que ya era verde,
// y el archivo y el historial guardaban ese amarillo hasta la lectura siguiente.
//
// Se evalúa y se escribe como siempre, y se vuelve a evaluar. Si la escritura
// movió el veredicto, la nota se escribe otra vez con el nuevo; la segunda
// escritura no agrega eventos, porque el eje del check ya no cambia. Sin motor
// de eventos, o cuando la escritura no mueve nada, es una sola escritura, como
// antes.
func EscribirConColor(path string, n *Note, vault map[string]*Note, contradictions map[string]bool, today Date) (Verdict, error) {
	v := Evaluate(n, vault, contradictions, today)
	n.Apply(v)
	if err := WriteNoteFile(path, n); err != nil {
		return Verdict{}, err
	}
	despues := Evaluate(n, vault, contradictions, today)
	if mismoVeredicto(v, despues) {
		return v, nil
	}
	n.Apply(despues)
	if err := WriteNoteFile(path, n); err != nil {
		return Verdict{}, err
	}
	return despues, nil
}

// mismoVeredicto compara por valor. Date lleva un time.Time adentro, y dos
// fechas iguales pueden no ser == como estructuras.
func mismoVeredicto(a, b Verdict) bool {
	return a.Color == b.Color && a.Reason == b.Reason && a.StaleAt.String() == b.StaleAt.String()
}
