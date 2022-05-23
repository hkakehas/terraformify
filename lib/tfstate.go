package terraformify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"os"
	"path/filepath"

	"github.com/itchyny/gojq"
)

type QueryParams struct {
	ResourceName  string
	AttributeType string
	Name          string
	Query         string
}

const serviceQueryTmpl =   `.resources[] | select(.name == "{{.ResourceName}}") | .instances[].attributes.{{.AttributeType}}[] | select(.name == "{{.Name}}") | .{{.Query}}`
const dsnippetQueryTmpl = `.resources[] | select(.name == "{{.ResourceName}}") | .instances[].attributes.content`
const setActivateQuery =   `(.resources[] | select(.type == "fastly_service_vcl" or .type == "fastly_service_waf_configuration") | .instances[].attributes.activate) |= true`
const setManageSnippetsQuery = `(.resources[] | select(.type == "fastly_service_dynamic_snippet_content") | .instances[].attributes.manage_snippets) |=true`
const setManageItemsQuery = `(.resources[] | select(.type == "fastly_service_dictionary_items") | .instances[].attributes.manage_items) |=true`
const setManageEntriesQuery = `(.resources[] | select(.type == "fastly_service_acl_entries") | .instances[].attributes.manage_entries) |=true`

type TFStateWithQueryTemplate struct {
	*template.Template
	*TFState
}
func (s *TFStateWithQueryTemplate) Query(params QueryParams) (*TFState, error) {
	var query bytes.Buffer
	if err := s.Execute(&query, params); err != nil {
		return nil, fmt.Errorf("tfstate: invalid params: %w", err)
	}

	return s.TFState.Query(query.String())
}

type TFState struct {
	Value     interface{}
}

func LoadTFState(workingDir string) (*TFState, error) {
	file := filepath.Join(workingDir, "terraform.tfstate")
	f, err := os.Open(file)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	var s TFState
	if err := json.NewDecoder(f).Decode(&s.Value); err != nil {
		return nil, fmt.Errorf("tfstate: invalid json: %w", err)
	}

	return &s, nil
}

func (s *TFState) addQueryTemplate(tmpl string) (*TFStateWithQueryTemplate, error) {
	t, err := template.New("template").Parse(tmpl)
	if err != nil {
		return nil, fmt.Errorf("tfstate: invalid template: %w", err)
	}

	return &TFStateWithQueryTemplate{t, s}, nil
}

func (s TFState) Bytes() []byte {
	switch v := (s.Value).(type) {
	case string:
		return []byte(v)
	default:
		b, _ := json.Marshal(v)
		return b
	}
}

func (s TFState) String() string {
	return string(s.Bytes())
}

func (s *TFState) Query(query string) (*TFState, error) {
	jq, err := gojq.Parse(query)
	if err != nil {
		return nil, err
	}
	iter := jq.Run(s.Value)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := v.(error); ok {
			return nil, err
		}
		return &TFState{Value: v}, nil
	}
	return nil, fmt.Errorf("tfstate: %s is not found in the state", query)
}

func (s *TFState) SetActivateAttr() (*TFState, error) {
	q :=  setActivateQuery
	return s.Query(q)
}

func (s *TFState) SetManageSnippetsAttr() (*TFState, error) {
	q :=  setManageSnippetsQuery
	return s.Query(q)
}

func (s *TFState) SetManageItemsAttr() (*TFState, error) {
	q :=  setManageItemsQuery
	return s.Query(q)
}

func (s *TFState) SetManageEntriesAttr() (*TFState, error) {
	q := setManageEntriesQuery
	return s.Query(q)
}

func (s *TFState) SetManageAttrs() (*TFState, error) {
	newState, err := s.SetManageEntriesAttr()
	if err != nil {
		return nil, err
	}
	newState, err = newState.SetManageItemsAttr()
	if err != nil {
		return nil, err
	}
	return newState.SetManageSnippetsAttr()
}