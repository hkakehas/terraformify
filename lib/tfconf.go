package terraformify

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

var (
	ErrAttrNotFound = errors.New("attribute not found")
)

type TFConf struct {
	*hclwrite.File
}

func LoadTFConf(rawHCL string) (*TFConf, error) {
	// "%" in log format conflicts with the HCL syntax.
	// Escaping it with an extra `%` to workaround the parser errro.
	rawHCL = strings.ReplaceAll(rawHCL, "%{", "%%{")
	// "terraform show" displays "(sensitive value)" for fieleds masked as sensitive, causing a parser error
	// Replacing them with quoted literals to workaround
	rawHCL = strings.ReplaceAll(rawHCL, " = (sensitive value)", ` = "(sensitive value)"`)

	f, diags := hclwrite.ParseConfig([]byte(rawHCL), "", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, fmt.Errorf("errors: %s", diags)
	}

	return &TFConf{f}, nil
}

func (tfconf *TFConf) ParseVCLServiceResource(serviceProp *VCLServiceResourceProp, c Config) ([]TFBlockProp, error) {
	// Check top-level blocks
	blocks := tfconf.Body().Blocks()
	if len(blocks) != 1 {
		return nil, fmt.Errorf("tfconf: Number of VCLServiceResourceProp should be 1, got %d", len(blocks))
	}
	block := blocks[0]

	if block.Type() != "resource" || block.Labels()[0] != serviceProp.GetType() {
		return nil, fmt.Errorf("tfconf: Unexpected Terraform block: %#v", block)
	}

	body := block.Body()

	// Get the nested blocks
	nestedBlocks := body.Blocks()

	// Collect block properties that require surgical changes.
	props := make([]TFBlockProp, 0, len(nestedBlocks))

	for _, block := range nestedBlocks {
		blockType := block.Type()

		switch blockType {
		case "acl":
			id, err := getStringAttributeValue(block, "acl_id")
			if err != nil {
				return nil, err
			}
			name, err := getStringAttributeValue(block, "name")
			if err != nil {
				return nil, err
			}
			prop := NewACLResourceProp(id, name, serviceProp)
			props = append(props, prop)
		case "dictionary":
			id, err := getStringAttributeValue(block, "dictionary_id")
			if err != nil {
				return nil, err
			}
			name, err := getStringAttributeValue(block, "name")
			if err != nil {
				return nil, err
			}
			prop := NewDictionaryResourceProp(id, name, serviceProp)
			props = append(props, prop)
		case "waf":
			id, err := getStringAttributeValue(block, "waf_id")
			if err != nil {
				return nil, err
			}
			prop := NewWAFResourceProp(id, serviceProp)
			props = append(props, prop)
		case "dynamicsnippet":
			id, err := getStringAttributeValue(block, "snippet_id")
			if err != nil {
				return nil, err
			}
			name, err := getStringAttributeValue(block, "name")
			if err != nil {
				return nil, err
			}
			prop := NewDynamicSnippetResourceProp(id, name, serviceProp)
			props = append(props, prop)
		}
	}

	return props, nil
}

func (tfconf *TFConf) RewriteResources(serviceProp *VCLServiceResourceProp, c Config) ([]byte, error) {
	// Read terraform.tfstate into the variable
	tfstate, err := LoadTFState(c.Directory)
	if err != nil {
		return nil, err
	}

	// Read resource blocks
	for _, block := range tfconf.Body().Blocks() {
		if t := block.Type(); t != "resource" {
			return nil, fmt.Errorf("Unexpected block type: %v\n", t)
		}
		switch block.Labels()[0] {
		case "fastly_service_vcl":
			err := rewriteVCLServiceResource(block, serviceProp, tfstate, c)
			if err != nil {
				return nil, err
			}
		case "fastly_service_waf_configuration":
			err := rewriteWAFResource(block, serviceProp)
			if err != nil {
				return nil, err
			}
		case "fastly_service_dynamic_snippet_content":
			err := rewriteDynamicSnippetResource(block, serviceProp, tfstate, c)
			if err != nil {
				return nil, err
			}
		case "fastly_service_dictionary_items":
			err := rewriteDictionaryResource(block, serviceProp, c)
			if err != nil {
				return nil, err
			}
		case "fastly_service_acl_entries":
			err := rewriteACLResource(block, serviceProp, c)
			if err != nil {
				return nil, err
			}
		}
	}
	return tfconf.Bytes(), nil
}

func rewriteVCLServiceResource(block *hclwrite.Block, serviceProp *VCLServiceResourceProp, s *TFState, c Config) error {
	tfstate, err := s.addQueryTemplate(serviceQueryTmpl)
	if err != nil {
		return err
	}

	// Remove read-only attributes
	body := block.Body()
	body.RemoveAttribute("id")
	body.RemoveAttribute("active_version")
	body.RemoveAttribute("cloned_version")

	// If no service level comments are set, set blank
	// Otherwise, Terraform will set `Managed by Terraform` and cause a configuration diff
	comment, err := getStringAttributeValue(block, "comment")
	if err != nil {
		if !errors.Is(err, ErrAttrNotFound) {
			return err
		}

		if comment == "" {
			// Set blank for the service-level comment, otherwise Terraform set `Managed by Terraform` by default causing config diffs.
			body.SetAttributeValue("comment", cty.StringVal(""))
		}
	}

	for _, block := range body.Blocks() {
		blockType := block.Type()
		nestedBlock := block.Body()

		switch blockType {
		case "acl":
			nestedBlock.RemoveAttribute("acl_id")
		case "dictionary":
			nestedBlock.RemoveAttribute("dictionary_id")
		case "waf":
			nestedBlock.RemoveAttribute("waf_id")
		case "dynamicsnippet":
			nestedBlock.RemoveAttribute("snippet_id")
		case "backend":
			name, err := getStringAttributeValue(block, "name")
			if err != nil {
				return err
			}
			keys := []string{"ssl_client_cert", "ssl_client_key"}

			for _, key := range keys {
				v, err := tfstate.Query(QueryParams{
					ResourceName:  serviceProp.GetNormalizedName(),
					AttributeType: blockType,
					Name:          name,
					Query:         key,
				})
				if err != nil {
					return err
				}
				if v.String() != "" {
					nestedBlock.SetAttributeValue(key, cty.StringVal(v.String()))
				}
			}
		case "request_setting":
			// Get name from TFConf
			name, err := getStringAttributeValue(block, "name")
			if err != nil {
				return err
			}

			// Get content from TFState
			v, err := tfstate.Query(QueryParams{
				ResourceName:  serviceProp.GetNormalizedName(),
				AttributeType: blockType,
				Name:          name,
				Query:         "xff",
			})
			if err != nil {
				return err
			}

			// In the provider schema, xff is an optional attribute with a default value of "append"
			// Because of the default value, Terraform attempts to add the default value even if the value is not set for the actual service.
			// To workaround the issue, explicitly setting xff attribute with blank value if it's blank in the state file
			if v.String() == "" {
				nestedBlock.SetAttributeValue("xff", cty.StringVal(""))
			}
		case "response_object":
			// Get name from TFConf
			name, err := getStringAttributeValue(block, "name")
			if err != nil {
				return err
			}

			// Get content from TFState
			v, err := tfstate.Query(QueryParams{
				ResourceName:  serviceProp.GetNormalizedName(),
				AttributeType: blockType,
				Name:          name,
				Query:         "content",
			})
			if err != nil {
				return err
			}

			ext := "txt"
			filename := fmt.Sprintf("%s.%s", normalize(name), ext)
			if err = saveContent(c.Directory, filename, v.Bytes()); err != nil {
				return err
			}

			// Replace content attribute of the nested block with file function expression
			path := fmt.Sprintf("./content/%s", filename)
			tokens := buildFileFunction(path)
			nestedBlock.SetAttributeRaw("content", tokens)
		case "snippet":
			// Get name from TFConf
			name, err := getStringAttributeValue(block, "name")
			if err != nil {
				return err
			}

			// Get content from TFState
			v, err := tfstate.Query(QueryParams{
				ResourceName:  serviceProp.GetNormalizedName(),
				AttributeType: blockType,
				Name:          name,
				Query:         "content",
			})
			if err != nil {
				return err
			}

			// Save content to a file
			filename := fmt.Sprintf("snippet_%s.vcl", normalize(name))
			if err = saveVCL(c.Directory, filename, v.Bytes()); err != nil {
				return err
			}

			// Replace content attribute of the nested block with file function expression
			path := fmt.Sprintf("./vcl/%s", filename)
			tokens := buildFileFunction(path)
			nestedBlock.SetAttributeRaw("content", tokens)
		case "vcl":
			// Get name from TFConf
			name, err := getStringAttributeValue(block, "name")
			if err != nil {
				return err
			}

			// Get content from TFState
			v, err := tfstate.Query(QueryParams{
				ResourceName:  serviceProp.GetNormalizedName(),
				AttributeType: blockType,
				Name:          name,
				Query:         "content",
			})
			if err != nil {
				return err
			}

			// Save content to a file
			filename := fmt.Sprintf("%s.vcl", normalize(name))
			if err = saveVCL(c.Directory, filename, v.Bytes()); err != nil {
				return err
			}

			// Replace content attribute of the nested block with file function expression
			path := fmt.Sprintf("./vcl/%s", filename)
			tokens := buildFileFunction(path)
			nestedBlock.SetAttributeRaw("content", tokens)
		default:
			if strings.HasPrefix(blockType, "logging_") {
				name, err := getStringAttributeValue(block, "name")
				if err != nil {
					return err
				}
				format, err := tfstate.Query(QueryParams{
					ResourceName:  serviceProp.GetNormalizedName(),
					AttributeType: blockType,
					Name:          name,
					Query:         "format",
				})
				ext := "txt"
				if json.Valid(format.Bytes()) {
					ext = "json"
				}
				filename := fmt.Sprintf("%s.%s", normalize(name), ext)
				if err = saveLogFormat(c.Directory, filename, format.Bytes()); err != nil {
					return err
				}
				// Replace content attribute of the nested block with file function expression
				path := fmt.Sprintf("./logformat/%s", filename)
				tokens := buildFileFunction(path)
				nestedBlock.SetAttributeRaw("format", tokens)

				// Populate sensitive attributes from the state file
				var keys []string
				switch blockType {
				case "logging_bigquery":
					keys = []string{"email", "secret_key"}
				case "logging_blobstorage":
					keys = []string{"sas_token"}
				case "logging_cloudfiles":
					keys = []string{"access_key"}
				case "logging_datadog":
					keys = []string{"token"}
				case "logging_digitalocean":
					keys = []string{"access_key", "secret_key"}
				case "logging_elasticsearch":
					keys = []string{"password", "tls_client_key"}
				case "logging_ftp":
					keys = []string{"password"}
				case "logging_gcs":
					keys = []string{"secret_key"}
				case "logging_googlepubsub":
					keys = []string{"secret_key"}
				case "logging_heroku":
					keys = []string{"token"}
				case "logging_honeycomb":
					keys = []string{"token"}
				case "logging_https":
					keys = []string{"tls_client_key"}
				case "logging_kafka":
					keys = []string{"password", "tls_client_key"}
				case "logging_kinesis":
					keys = []string{"access_key", "secret_key"}
				case "logging_loggly":
					keys = []string{"token"}
				case "logging_logshuttle":
					keys = []string{"token"}
				case "logging_newrelic":
					keys = []string{"token"}
				case "logging_openstack":
					keys = []string{"access_key"}
				case "logging_s3":
					keys = []string{"s3_access_key", "s3_secret_key"}
				case "logging_scalyr":
					keys = []string{"token"}
				case "logging_sftp":
					keys = []string{"password", "secret_key"}
				case "logging_splunk":
					keys = []string{"tls_client_key", "token"}
				case "logging_syslog":
					keys = []string{"tls_client_key"}
				}
				for _, key := range keys {
					v, err := tfstate.Query(QueryParams{
						ResourceName:  serviceProp.GetNormalizedName(),
						AttributeType: blockType,
						Name:          name,
						Query:         key,
					})
					if err != nil {
						return err
					}
					nestedBlock.SetAttributeValue(key, cty.StringVal(v.String()))
				}
			}
		}
	}
	return nil
}

func rewriteACLResource(block *hclwrite.Block, serviceProp *VCLServiceResourceProp, c Config) error {
	body := block.Body()
	// remove read-only attributes
	body.RemoveAttribute("id")

	if c.ManageAll {
		// set manage_entries to true
		body.SetAttributeValue("manage_entries", cty.BoolVal(true))
	}

	// set service_id to represent the resource dependency
	ref := buildServiceIDRef(serviceProp)
	body.SetAttributeTraversal("service_id", ref)

	// remove read-only attributes from each ACL entry
	for _, block := range body.Blocks() {
		t := block.Type()
		nb := block.Body()
		if t != "entry" {
			return fmt.Errorf("Unexpected Terraform block: %#v", block)
		}
		nb.RemoveAttribute("id")
	}

	return nil
}

func rewriteDictionaryResource(block *hclwrite.Block, serviceProp *VCLServiceResourceProp, c Config) error {
	body := block.Body()
	// remove read-only attributes
	body.RemoveAttribute("id")

	if c.ManageAll {
		// set manage_items to true
		body.SetAttributeValue("manage_items", cty.BoolVal(true))
	}

	// set service_id to represent the resource dependency
	ref := buildServiceIDRef(serviceProp)
	body.SetAttributeTraversal("service_id", ref)

	return nil
}

func rewriteWAFResource(block *hclwrite.Block, serviceProp *VCLServiceResourceProp) error {
	body := block.Body()
	// remove read-only attributes
	body.RemoveAttribute("active")
	body.RemoveAttribute("cloned_version")
	body.RemoveAttribute("number")
	body.RemoveAttribute("id")

	// set waf_id to represent the resource dependency
	body.SetAttributeTraversal("waf_id", hcl.Traversal{
		hcl.TraverseRoot{Name: serviceProp.GetType()},
		hcl.TraverseAttr{Name: serviceProp.GetNormalizedName()},
		hcl.TraverseAttr{Name: "waf"},
		hcl.TraverseIndex{Key: cty.NumberUIntVal(0)},
		hcl.TraverseAttr{Name: "waf_id"},
	})

	return nil
}

func rewriteDynamicSnippetResource(block *hclwrite.Block, serviceProp *VCLServiceResourceProp, s *TFState, c Config) error {
	tfstate, err := s.addQueryTemplate(dsnippetQueryTmpl)
	if err != nil {
		return err
	}

	body := block.Body()
	// remove read-only attributes
	body.RemoveAttribute("id")

	if c.ManageAll {
		// set manage_snippets to true
		body.SetAttributeValue("manage_snippets", cty.BoolVal(true))
	}

	// set service_id to represent the resource dependency
	ref := buildServiceIDRef(serviceProp)
	body.SetAttributeTraversal("service_id", ref)

	// replace content value with file()
	name := block.Labels()[1]

	// Get content from TFState
	v, err := tfstate.Query(QueryParams{
		ResourceName: name,
	})
	if err != nil {
		return err
	}

	// Save content to a file
	filename := fmt.Sprintf("dsnippet_%s.vcl", normalize(name))
	if err = saveVCL(c.Directory, filename, v.Bytes()); err != nil {
		return err
	}

	// Replace content attribute with file function expression
	path := fmt.Sprintf("./vcl/%s", filename)
	tokens := buildFileFunction(path)
	body.SetAttributeRaw("content", tokens)

	return nil
}

func buildFileFunction(path string) hclwrite.Tokens {
	return hclwrite.Tokens{
		{Type: hclsyntax.TokenIdent, Bytes: []byte("file")},
		{Type: hclsyntax.TokenOParen, Bytes: []byte{'('}},
		{Type: hclsyntax.TokenOQuote, Bytes: []byte{'"'}},
		{Type: hclsyntax.TokenQuotedLit, Bytes: []byte(path)},
		{Type: hclsyntax.TokenCQuote, Bytes: []byte{'"'}},
		{Type: hclsyntax.TokenCParen, Bytes: []byte{')'}},
	}
}

func buildServiceIDRef(serviceProp *VCLServiceResourceProp) hcl.Traversal {
	return hcl.Traversal{
		hcl.TraverseRoot{Name: serviceProp.GetType()},
		hcl.TraverseAttr{Name: serviceProp.GetNormalizedName()},
		hcl.TraverseAttr{Name: "id"},
	}
}

func getStringAttributeValue(block *hclwrite.Block, attrKey string) (string, error) {
	// find TokenQuotedLit
	attr := block.Body().GetAttribute(attrKey)
	if attr == nil {
		return "", fmt.Errorf(`%w: failed to find "%s" in "%s"`, ErrAttrNotFound, attrKey, block.Type())
	}

	expr := attr.Expr()
	exprTokens := expr.BuildTokens(nil)

	i := 0
	for i < len(exprTokens) && exprTokens[i].Type != hclsyntax.TokenQuotedLit {
		i++
	}

	if i == len(exprTokens) {
		return "", fmt.Errorf("failed to find TokenQuotedLit: %#v", attr)
	}

	value := string(exprTokens[i].Bytes)
	return value, nil
}

func saveContent(workingDir, name string, content []byte) error {
	return saveFile(workingDir, name, "content", content)
}

func saveVCL(workingDir, name string, content []byte) error {
	return saveFile(workingDir, name, "vcl", content)
}

func saveLogFormat(workingDir, name string, content []byte) error {
	return saveFile(workingDir, name, "logformat", content)
}

func saveFile(workingDir, name, fileType string, content []byte) error {
	dir := filepath.Join(workingDir, fileType)
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(dir, 0755)
		if err != nil {
			return err
		}
	}

	file := filepath.Join(workingDir, fileType, name)
	return os.WriteFile(file, content, 0644)
}
