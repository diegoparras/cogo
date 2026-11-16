// Package suasion holds COGO's anti-manipulation ("autonomy") engine: an ontology
// of influence, coercion and rhetorical techniques across six adversarial
// disciplines, used to read a model turn for pressure on the human's autonomy
// (their declared "mandate"). See docs/motor-autonomia.md.
//
// This file only loads and validates the embedded ontology; the detection
// pipeline is built on top of it in later phases.
package suasion

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed ontology/*.yaml
var ontologyFS embed.FS

// Enumerations enforced by Validate.
var (
	validAxes        = map[string]bool{"presion": true, "autonomia": true, "asimetria": true, "veracidad": true}
	validTrajectory  = map[string]bool{"single": true, "longitudinal": true, "both": true}
	validSeverity    = map[string]bool{"low": true, "medium": true, "high": true, "critical": true}
	validDetector    = map[string]bool{"lexicon": true, "speech_act": true, "pragmatic": true, "structure": true, "trajectory": true, "nli_transcript": true, "frame": true, "meta": true}
	validDisciplines = map[string]bool{"persuasion": true, "interrogation": true, "negotiation": true, "coercion": true, "dark_psychology": true, "rhetoric": true}
)

// Source is an academic citation. Each subfield is optional so an entry can omit
// what is uncertain rather than fabricate it. Note records real provenance when
// no single citable work exists (e.g. popular-literature terms).
type Source struct {
	Author string `yaml:"author,omitempty"`
	Work   string `yaml:"work,omitempty"`
	Year   int    `yaml:"year,omitempty"`
	Note   string `yaml:"note,omitempty"`
}

// Detector is one way a technique fires. Type is from the detector enum.
type Detector struct {
	Type   string `yaml:"type"`
	Signal string `yaml:"signal"`
}

// Countermeasure is the paired defensive doctrine surfaced to the human.
type Countermeasure struct {
	Doctrine    string `yaml:"doctrine,omitempty"`
	Move        string `yaml:"move"`
	Inoculation string `yaml:"inoculation"`
}

// Technique is one record in the ontology.
type Technique struct {
	ID                string         `yaml:"id"`
	Name              string         `yaml:"name"`
	AKA               []string       `yaml:"aka,omitempty"`
	Family            string         `yaml:"family,omitempty"`
	Source            Source         `yaml:"source"`
	Definition        string         `yaml:"definition"`
	Mechanism         string         `yaml:"mechanism"`
	InLLM             string         `yaml:"in_llm"`
	Axes              []string       `yaml:"axes"`
	Detectors         []Detector     `yaml:"detectors"`
	Trajectory        string         `yaml:"trajectory"`
	Severity          string         `yaml:"severity"`
	CriticalQuestions []string       `yaml:"critical_questions"`
	Countermeasure    Countermeasure `yaml:"countermeasure"`
	FPGuard           string         `yaml:"fp_guard,omitempty"`
	MapsTo            []string       `yaml:"maps_to,omitempty"`
}

// Discipline is one ontology file.
type Discipline struct {
	Discipline  string      `yaml:"discipline"`
	DisplayName string      `yaml:"display_name"`
	Techniques  []Technique `yaml:"techniques"`
}

// Ontology is the loaded set of all disciplines, indexed by technique id.
type Ontology struct {
	Disciplines []Discipline
	byID        map[string]*Technique
}

// Load reads and parses every discipline file embedded under ontology/. Files
// whose basename starts with "_" are metadata and skipped.
func Load() (*Ontology, error) {
	entries, err := fs.ReadDir(ontologyFS, "ontology")
	if err != nil {
		return nil, err
	}
	o := &Ontology{byID: map[string]*Technique{}}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".yaml") || strings.HasPrefix(name, "_") {
			continue
		}
		b, err := ontologyFS.ReadFile("ontology/" + name)
		if err != nil {
			return nil, err
		}
		var d Discipline
		if err := yaml.Unmarshal(b, &d); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		o.Disciplines = append(o.Disciplines, d)
	}
	for di := range o.Disciplines {
		for ti := range o.Disciplines[di].Techniques {
			t := &o.Disciplines[di].Techniques[ti]
			o.byID[t.ID] = t
		}
	}
	return o, nil
}

// Get returns a technique by id, or nil.
func (o *Ontology) Get(id string) *Technique { return o.byID[id] }

// Len returns the number of techniques.
func (o *Ontology) Len() int { return len(o.byID) }

// Validate checks schema integrity: enums, required fields, unique ids, and that
// every maps_to target resolves. It reports all problems at once.
func (o *Ontology) Validate() error {
	var probs []string
	seen := map[string]string{} // id -> discipline
	for _, d := range o.Disciplines {
		if !validDisciplines[d.Discipline] {
			probs = append(probs, fmt.Sprintf("discipline %q not in enum", d.Discipline))
		}
		for _, t := range d.Techniques {
			if t.ID == "" {
				probs = append(probs, d.Discipline+": technique with empty id")
				continue
			}
			if prev, ok := seen[t.ID]; ok {
				probs = append(probs, fmt.Sprintf("%s: duplicate id (also in %s)", t.ID, prev))
			}
			seen[t.ID] = d.Discipline
			if !strings.HasPrefix(t.ID, d.Discipline+".") {
				probs = append(probs, t.ID+": id should be prefixed with its discipline")
			}
			for _, req := range []struct{ k, v string }{
				{"name", t.Name}, {"definition", t.Definition}, {"mechanism", t.Mechanism},
				{"in_llm", t.InLLM}, {"trajectory", t.Trajectory}, {"severity", t.Severity},
				{"countermeasure.move", t.Countermeasure.Move},
				{"countermeasure.inoculation", t.Countermeasure.Inoculation},
			} {
				if strings.TrimSpace(req.v) == "" {
					probs = append(probs, t.ID+": missing "+req.k)
				}
			}
			if len(t.Axes) == 0 {
				probs = append(probs, t.ID+": no axes")
			}
			for _, a := range t.Axes {
				if !validAxes[a] {
					probs = append(probs, t.ID+": axis "+a+" not in enum")
				}
			}
			if len(t.Detectors) == 0 {
				probs = append(probs, t.ID+": no detectors")
			}
			for _, det := range t.Detectors {
				if !validDetector[det.Type] {
					probs = append(probs, t.ID+": detector type "+det.Type+" not in enum")
				}
			}
			if !validTrajectory[t.Trajectory] {
				probs = append(probs, t.ID+": trajectory "+t.Trajectory+" not in enum")
			}
			if !validSeverity[t.Severity] {
				probs = append(probs, t.ID+": severity "+t.Severity+" not in enum")
			}
			if len(t.CriticalQuestions) == 0 {
				probs = append(probs, t.ID+": no critical_questions")
			}
		}
	}
	for _, d := range o.Disciplines {
		for _, t := range d.Techniques {
			for _, m := range t.MapsTo {
				if _, ok := seen[m]; !ok {
					probs = append(probs, t.ID+": maps_to unknown id "+m)
				}
			}
		}
	}
	if len(probs) > 0 {
		sort.Strings(probs)
		return fmt.Errorf("ontology invalid (%d problems):\n  %s", len(probs), strings.Join(probs, "\n  "))
	}
	return nil
}
