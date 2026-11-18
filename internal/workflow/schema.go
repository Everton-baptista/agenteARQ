package workflow

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	agentarch "github.com/Everton-baptista/agenteARQ"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

var compiledSchemas sync.Map

func validateArtifact(raw []byte, name string) error {
	var schema *jsonschema.Schema
	if cached, ok := compiledSchemas.Load(name); ok {
		schema = cached.(*jsonschema.Schema)
	} else {
		b, e := agentarch.Spec.ReadFile("spec/schemas/" + name)
		if e != nil {
			return e
		}
		doc, e := jsonschema.UnmarshalJSON(strings.NewReader(string(b)))
		if e != nil {
			return e
		}
		c := jsonschema.NewCompiler()
		if e = c.AddResource(name, doc); e != nil {
			return e
		}
		schema, e = c.Compile(name)
		if e != nil {
			return e
		}
		compiledSchemas.Store(name, schema)
	}
	var value any
	if e := yaml.Unmarshal(raw, &value); e != nil {
		return e
	}
	b, e := json.Marshal(value)
	if e != nil {
		return e
	}
	doc, e := jsonschema.UnmarshalJSON(strings.NewReader(string(b)))
	if e != nil {
		return e
	}
	if e = schema.Validate(doc); e != nil {
		return fmt.Errorf("artifact does not match %s: %w", name, e)
	}
	return nil
}
