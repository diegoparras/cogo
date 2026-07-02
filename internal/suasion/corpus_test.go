package suasion

import (
	"os"
	"testing"

	"github.com/diegoparras/cogo/internal/core"
	"gopkg.in/yaml.v3"
)

// The calibration harness. It runs the DETERMINISTIC engine (no model, so the
// results are reproducible in CI) over a labeled corpus and reports the metric
// that actually matters: the false-positive rate on benign text. It is not an
// unbiased benchmark — see testdata/corpus.yaml — but it makes the engine's
// behavior measurable and guards against regressions.

type corpusCase struct {
	ID         string   `yaml:"id"`
	Kind       string   `yaml:"kind"`
	Turn       string   `yaml:"turn"`
	Transcript []Turn   `yaml:"transcript"`
	Mandate    *Mandate `yaml:"mandate"`
	Expect     struct {
		MaxColor  string `yaml:"max_color"`
		MinColor  string `yaml:"min_color"`
		Technique string `yaml:"technique"`
	} `yaml:"expect"`
}

func colorByName(s string) core.Color {
	switch s {
	case "green":
		return core.Green
	case "yellow":
		return core.Yellow
	case "red":
		return core.Red
	}
	return core.Ungraded
}

func loadCorpus(t *testing.T) []corpusCase {
	t.Helper()
	b, err := os.ReadFile("testdata/corpus.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Cases []corpusCase `yaml:"cases"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Cases) == 0 {
		t.Fatal("empty corpus")
	}
	return doc.Cases
}

func TestCalibration(t *testing.T) {
	e := testEngine(t)
	cases := loadCorpus(t)

	var (
		benign, benignGreen, benignRed, benignSoftFP int
		manip, manipMissed, techWanted, techMissed   int
	)

	for _, c := range cases {
		r := e.Analyze(c.Turn, c.Transcript, c.Mandate) // deterministic path
		hasTech := func(id string) bool {
			for _, f := range r.Findings {
				if f.TechniqueID == id {
					return true
				}
			}
			return false
		}

		switch c.Kind {
		case "benign":
			benign++
			max := colorByName(c.Expect.MaxColor)
			if r.Overall == core.Green {
				benignGreen++
			}
			if r.Overall == core.Red {
				benignRed++ // the expensive false positive
			}
			if r.Overall > max {
				if r.Overall == core.Red {
					t.Errorf("[%s] FALSE POSITIVE (red): benign turn flagged red — %s", c.ID, r.Reason)
				} else {
					t.Errorf("[%s] over-flagged: got %v, want <= %v — %s", c.ID, r.Overall, max, r.Reason)
				}
			}
			// A trap benign (max=yellow) that goes yellow is an acceptable soft FP.
			if max == core.Yellow && r.Overall == core.Yellow {
				benignSoftFP++
			}

		case "manipulative":
			manip++
			min := colorByName(c.Expect.MinColor)
			if r.Overall < min {
				manipMissed++
				t.Errorf("[%s] MISSED: got %v, want >= %v — %s", c.ID, r.Overall, min, r.Reason)
			}
			if c.Expect.Technique != "" {
				techWanted++
				if !hasTech(c.Expect.Technique) {
					techMissed++
					var got []string
					for _, f := range r.Findings {
						got = append(got, f.TechniqueID)
					}
					t.Errorf("[%s] technique %q did not fire; got %v", c.ID, c.Expect.Technique, got)
				}
			}
		default:
			t.Fatalf("[%s] unknown kind %q", c.ID, c.Kind)
		}
	}

	// The honest report — printed on every run (go test -v).
	t.Logf("=== calibración (motor determinista) ===")
	t.Logf("benignos: %d | verdes (cero señal): %d | rojos (FP caro): %d | amarillos-trampa (FP blando aceptable): %d",
		benign, benignGreen, benignRed, benignSoftFP)
	t.Logf("manipulativos: %d | no atrapados: %d | técnica esperada fallada: %d/%d",
		manip, manipMissed, techMissed, techWanted)
	t.Logf("FALSOS POSITIVOS ROJOS: %d/%d (la métrica que más cuesta)", benignRed, benign)
	t.Logf("recall (alcanzan su color mínimo): %d/%d", manip-manipMissed, manip)
}
