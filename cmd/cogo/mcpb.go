package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// `cogo mcpb` empaqueta un binario de COGO como MCP Bundle: el .mcpb que se
// instala en Claude Desktop con un clic. Es un zip con el manifiesto y el
// binario adentro; Claude Desktop lo arranca por stdio sobre la carpeta que la
// persona elige como vault.
//
// El binario se empaqueta a sí mismo, o al de otra plataforma con -binario y
// -os: así el job de release arma los cinco .mcpb con el binario de linux. Las
// tools del manifiesto salen del propio servidor, así que no pueden quedar
// viejas.
func cmdMCPB(args []string) error {
	fs := flag.NewFlagSet("mcpb", flag.ExitOnError)
	binario := fs.String("binario", "", "binario a empaquetar (default: este mismo)")
	goos := fs.String("os", runtime.GOOS, "sistema del binario: linux, darwin o windows")
	arch := fs.String("arch", runtime.GOARCH, "arquitectura del binario, solo para el nombre del archivo")
	ver := fs.String("version", version, "versión del bundle, semver; acepta la v del tag")
	out := fs.String("o", "", "archivo de salida (default: cogo-<os>-<arch>.mcpb)")
	_ = fs.Parse(args)

	v := strings.TrimPrefix(*ver, "v")
	if !semver.MatchString(v) {
		return fmt.Errorf("-version %q: un .mcpb necesita una versión semver, como 0.2.0", *ver)
	}
	if *binario == "" {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		*binario = exe
	}
	if *out == "" {
		*out = fmt.Sprintf("cogo-%s-%s.mcpb", *goos, *arch)
	}
	tools, err := herramientasCOGO()
	if err != nil {
		return err
	}
	manifiesto, err := manifestMCPB(v, *goos, tools)
	if err != nil {
		return err
	}
	if err := escribirMCPB(*out, manifiesto, *binario, binarioMCPB(*goos)); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "cogo: %s (%s, %d tools)\n", *out, plataformaMCPB(*goos), len(tools))
	return nil
}

var semver = regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)

// plataformaMCPB traduce GOOS al nombre que usa el manifiesto.
func plataformaMCPB(goos string) string {
	if goos == "windows" {
		return "win32"
	}
	return goos
}

// binarioMCPB es dónde queda el binario adentro del bundle. El .exe va
// explícito en Windows, como en el ejemplo oficial de la spec.
func binarioMCPB(goos string) string {
	if goos == "windows" {
		return "server/cogo.exe"
	}
	return "server/cogo"
}

// herramientasCOGO lista las tools del servidor, con un vault vacío y en
// memoria: son las mismas que ve cualquier cliente.
func herramientasCOGO() ([]*mcp.Tool, error) {
	dir, err := os.MkdirTemp("", "cogo-mcpb-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	clientT, serverT := mcp.NewInMemoryTransports()
	go func() { _ = newMCPServer(dir).Run(ctx, serverT) }()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "cogo-mcpb", Version: version}, nil).Connect(ctx, clientT, nil)
	if err != nil {
		return nil, err
	}
	defer cs.Close()
	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		return nil, err
	}
	return res.Tools, nil
}

// manifestMCPB arma el manifest.json del bundle de una plataforma, según la
// versión 0.3 del manifiesto de MCP Bundles.
func manifestMCPB(ver, goos string, tools []*mcp.Tool) ([]byte, error) {
	type herramienta struct {
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
	}
	lista := make([]herramienta, 0, len(tools))
	for _, t := range tools {
		lista = append(lista, herramienta{Name: t.Name, Description: t.Description})
	}
	bin := binarioMCPB(goos)
	m := map[string]any{
		"manifest_version": "0.3",
		"name":             "cogo",
		"display_name":     "COGO",
		"version":          ver,
		"description":      "La memoria del agente con semáforo: cada nota con un color de confianza computado desde su evidencia.",
		"long_description": "COGO guarda lo que el agente sabe como notas con un color de confianza que se calcula desde la evidencia: " +
			"verde se puede apoyar, amarillo con cuidado, rojo no. Antes de algo costoso o irreversible, `authorize` dice si lo " +
			"que se sabe alcanza, y si no alcanza, te pregunta a vos. Este bundle corre COGO en tu máquina, sobre la carpeta que " +
			"elijas como vault.",
		"author":        map[string]any{"name": "Diego Parrás", "url": "https://github.com/diegoparras"},
		"repository":    map[string]any{"type": "git", "url": "https://github.com/diegoparras/cogo"},
		"homepage":      "https://github.com/diegoparras/cogo",
		"documentation": "https://github.com/diegoparras/cogo/blob/main/docs/manual.md",
		"support":       "https://github.com/diegoparras/cogo/issues",
		"license":       "MIT",
		"keywords":      []string{"memory", "agents", "trust", "evidence", "authorization"},
		"server": map[string]any{
			"type":        "binary",
			"entry_point": bin,
			"mcp_config": map[string]any{
				"command": "${__dirname}/" + bin,
				"args":    []string{"serve", "-vault", "${user_config.vault}"},
			},
		},
		"user_config": map[string]any{
			"vault": map[string]any{
				"type":        "directory",
				"title":       "Vault",
				"description": "La carpeta donde COGO guarda las notas. Si está vacía, COGO arranca un vault nuevo ahí.",
				"required":    true,
				"default":     "${HOME}/cogo-vault",
			},
		},
		"compatibility": map[string]any{"platforms": []string{plataformaMCPB(goos)}},
		"tools":         lista,
	}
	return json.MarshalIndent(m, "", "  ")
}

// escribirMCPB arma el zip: el manifiesto en la raíz y el binario en server/,
// con permiso de ejecución para macOS y Linux.
func escribirMCPB(salida string, manifiesto []byte, binario, adentro string) error {
	src, err := os.Open(binario)
	if err != nil {
		return fmt.Errorf("binario: %w", err)
	}
	defer src.Close()
	f, err := os.Create(salida)
	if err != nil {
		return err
	}
	z := zip.NewWriter(f)
	escribir := func(nombre string, modo os.FileMode, r io.Reader) error {
		h := &zip.FileHeader{Name: nombre, Method: zip.Deflate, Modified: time.Now().UTC()}
		h.SetMode(modo)
		w, err := z.CreateHeader(h)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, r)
		return err
	}
	if err := escribir("manifest.json", 0o644, strings.NewReader(string(manifiesto))); err != nil {
		f.Close()
		return err
	}
	if err := escribir(adentro, 0o755, src); err != nil {
		f.Close()
		return err
	}
	if err := z.Close(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
