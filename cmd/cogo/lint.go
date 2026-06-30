package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/lint"
	"github.com/diegoparras/cogo/internal/llm"
)

func cmdLint(args []string) error {
	fs := flag.NewFlagSet("lint", flag.ExitOnError)
	dir := vaultFlag(fs)
	_ = fs.Parse(args)

	vault, err := core.LoadVault(*dir)
	if err != nil {
		return err
	}
	p := llm.FromEnv()
	r := lint.Run(context.Background(), vault, today(), p)

	if p.Available() {
		fmt.Printf("llm: %s — %d/%d candidate pairs checked for contradictions\n", p.Name(), r.PairsChecked, r.CandidatePairs)
	} else {
		fmt.Println("llm: off — set COGO_LLM_BASE_URL + COGO_LLM_MODEL to enable contradiction detection")
	}

	if len(r.Issues) == 0 {
		fmt.Println("no issues found")
		return nil
	}
	for _, is := range r.Issues {
		fmt.Printf("- [%s] %s\n", is.Kind, is.Msg)
	}
	if c := r.Contradictions(); len(c) > 0 {
		fmt.Printf("\n%d note(s) would turn red from contradictions once colored.\n", len(c))
	}
	return nil
}
