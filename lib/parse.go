package terraformify

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

func parseHCL(rawHCL string) (*hclwrite.File, error) {
	// "%" in log format conflicts with the HCL syntax.
	// To make hclwrite.ParseConfig() work, it must be escaped with an extra "%"
	rawHCL = strings.ReplaceAll(rawHCL, "%{", "%%{")
	// Terraform masks values marked as sensitive with `(sensitive value)`, which causes parser errors.
	// Making them quoted string literals to workaround the errors.
	rawHCL = strings.ReplaceAll(rawHCL, "(sensitive value)", `"(sensitive value)"`)

	f, diags := hclwrite.ParseConfig([]byte(rawHCL), "", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, fmt.Errorf("errors: %s", diags)
	}

	return f, nil
}

func ParseVCLServiceResource(serviceProp *VCLServiceResourceProp, rawHCL string) ([]TFBlockProp, error) {
	f, err := parseHCL(rawHCL)
	if err != nil {
		return nil, err
	}

	// Check top-level blocks
	blocks := f.Body().Blocks()
	if len(blocks) != 1 {
		return nil, fmt.Errorf("Number of VCLServiceResourceProp should be 1, got %d", len(blocks))
	}
	block := blocks[0]

	if block.Type() != "resource" || block.Labels()[0] != serviceProp.GetType() {
		return nil, fmt.Errorf("Unexpected Terraform block: %#v", block)
	}

	// Get the nested blocks
	nestedBlocks := block.Body().Blocks()

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
		case "backend":
			name, err := getStringAttributeValue(block, "name")
			if err != nil {
				return nil, err
			}
			prop := NewBackendBlockProp(name, serviceProp)
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
		case "snippet":
			name, err := getStringAttributeValue(block, "name")
			if err != nil {
				return nil, err
			}
			prop := NewSnippetBlockProp(name, serviceProp)
			props = append(props, prop)
		case "vcl":
			name, err := getStringAttributeValue(block, "name")
			if err != nil {
				return nil, err
			}
			prop := NewVCLBlockProp(name, serviceProp)
			props = append(props, prop)
		default:
			if strings.HasPrefix(blockType, "logging_") {
				name, err := getStringAttributeValue(block, "name")
				if err != nil {
					return nil, err
				}
				prop := NewLoggingBlockProp(name, blockType, serviceProp)
				props = append(props, prop)
				continue
			}
			// PlaceholderProps are used to align the indexes of the blocks stored in the slice
			// between ParseVCLServiceResource and rewriteVCLServiceResource.
			prop := NewPlaceholderProp(serviceProp)
			props = append(props, prop)
		}
	}
	return props, nil
}

func RewriteResources(rawHCL string, serviceProp *VCLServiceResourceProp, props []TFBlockProp) ([]byte, error) {
	f, err := parseHCL(rawHCL)
	if err != nil {
		return nil, err
	}

	// Read resource blocks
	for _, block := range f.Body().Blocks() {
		if t := block.Type(); t != "resource" {
			return nil, fmt.Errorf("Unexpected block type: %v\n", t)
		}
		switch block.Labels()[0] {
		case "fastly_service_vcl":
			rewriteVCLServiceResource(block, serviceProp, props)
			if err != nil {
				return nil, err
			}
		case "fastly_service_waf_configuration":
			rewriteWAFResource(block, serviceProp)
			if err != nil {
				return nil, err
			}
		case "fastly_service_dynamic_snippet_content":
			rewriteDynamicSnippetResource(block, serviceProp)
			if err != nil {
				return nil, err
			}
		case "fastly_service_dictionary_items":
			rewriteDictionaryResource(block, serviceProp)
			if err != nil {
				return nil, err
			}
		case "fastly_service_acl_entries":
			rewriteACLResource(block, serviceProp)
			if err != nil {
				return nil, err
			}
		}
	}
	return f.Bytes(), nil
}

func rewriteVCLServiceResource(block *hclwrite.Block, serviceProp *VCLServiceResourceProp, props []TFBlockProp) error {
	body := block.Body()

	// Remove read-only attributes
	body.RemoveAttribute("id")
	body.RemoveAttribute("active_version")
	body.RemoveAttribute("cloned_version")

	for i, block := range body.Blocks() {
		blockType := block.Type()
		nestedBlock := block.Body()

		switch blockType {
		case "acl":
			nestedBlock.RemoveAttribute("acl_id")
		case "backend":
			prop, ok := props[i].(*BackendBlockProp)
			if !ok {
				return fmt.Errorf("Expected *BackendBlockProp, got %#v", props[i])
			}

			cert, ok := prop.SensitiveValues["ssl_client_cert"]
			if !ok {
				return fmt.Errorf("Sensitive value not found for the backend, %s", prop.Name)
			}
			if cert != "" {
				nestedBlock.SetAttributeValue("ssl_client_cert", cty.StringVal(cert))
			}

			key, ok := prop.SensitiveValues["ssl_client_key"]
			if !ok {
				return fmt.Errorf("Sensitive value not found for the backend, %s", prop.Name)
			}
			if key != "" {
				nestedBlock.SetAttributeValue("ssl_client_cert", cty.StringVal(key))
			}
		case "dictionary":
			nestedBlock.RemoveAttribute("dictionary_id")
		case "waf":
			nestedBlock.RemoveAttribute("waf_id")
		case "dynamicsnippet":
			nestedBlock.RemoveAttribute("snippet_id")
		case "snippet":
			// replace content value with file()
			name, err := getStringAttributeValue(block, "name")
			if err != nil {
				return err
			}
			name = normalizeName(name)
			path := fmt.Sprintf("./vcl/snippet_%s.vcl", name)
			tokens := getFileFunction(path)
			nestedBlock.SetAttributeRaw("content", tokens)
		case "vcl":
			// replace content value with file()
			name, err := getStringAttributeValue(block, "name")
			if err != nil {
				return err
			}
			name = normalizeName(name)
			path := fmt.Sprintf("./vcl/%s.vcl", name)
			tokens := getFileFunction(path)
			nestedBlock.SetAttributeRaw("content", tokens)
		default:
			if strings.HasPrefix(blockType, "logging_") {
				prop, ok := props[i].(*LoggingBlockProp)
				if !ok {
					return fmt.Errorf("Expected *LoggingBlockProp, got %#v", props[i])
				}

				name, err := getStringAttributeValue(block, "name")
				if err != nil {
					return err
				}

				name = normalizeName(name)
				ext := "txt"
				if prop.IsJSON {
					ext = "json"
				}
				path := fmt.Sprintf("./logformat/%s.%s", name, ext)
				tokens := getFileFunction(path)
				nestedBlock.SetAttributeRaw("format", tokens)

				switch prop.GetEndpointType() {
				case "logging_bigquery":
					value, ok := prop.SensitiveValues["bigquery_email"]
					if !ok {
						return fmt.Errorf("Sensitive value not found for %s", prop.GetEndpointType())
					}
					nestedBlock.SetAttributeValue("email", cty.StringVal(value))
					value, ok = prop.SensitiveValues["bigquery_secret_key"]
					if !ok {
						return fmt.Errorf("Sensitive value not found for %s", prop.GetEndpointType())
					}
					nestedBlock.SetAttributeValue("secret_key", cty.StringVal(value))
				case "logging_blobstorage":
					value, ok := prop.SensitiveValues["blobstorage_sas_token"]
					if !ok {
						return fmt.Errorf("Sensitive value not found for %s", prop.GetEndpointType())
					}
					nestedBlock.SetAttributeValue("sas_token", cty.StringVal(value))
				case "logging_cloudfiles":
					value, ok := prop.SensitiveValues["cloudfiles_access_key"]
					if !ok {
						return fmt.Errorf("Sensitive value not found for %s", prop.GetEndpointType())
					}
					nestedBlock.SetAttributeValue("access_key", cty.StringVal(value))
				case "logging_datadog":
					value, ok := prop.SensitiveValues["datadog_token"]
					if !ok {
						return fmt.Errorf("Sensitive value not found for %s", prop.GetEndpointType())
					}
					nestedBlock.SetAttributeValue("token", cty.StringVal(value))
				case "logging_digitalocean":
					value, ok := prop.SensitiveValues["digitalocean_access_key"]
					if !ok {
						return fmt.Errorf("Sensitive value not found for %s", prop.GetEndpointType())
					}
					nestedBlock.SetAttributeValue("access_key", cty.StringVal(value))
					value, ok = prop.SensitiveValues["digitalocean_secret_key"]
					if !ok {
						return fmt.Errorf("Sensitive value not found for %s", prop.GetEndpointType())
					}
					nestedBlock.SetAttributeValue("secret_key", cty.StringVal(value))
				case "logging_elasticsearch":
					value, ok := prop.SensitiveValues["elasticsearch_password"]
					if !ok {
						return fmt.Errorf("Sensitive value not found for %s", prop.GetEndpointType())
					}
					nestedBlock.SetAttributeValue("password", cty.StringVal(value))
					value, ok = prop.SensitiveValues["elasticsearch_tls_client_key"]
					if !ok {
						return fmt.Errorf("Sensitive value not found for %s", prop.GetEndpointType())
					}
					nestedBlock.SetAttributeValue("tls_client_key", cty.StringVal(value))
				case "logging_ftp":
					value, ok := prop.SensitiveValues["ftp_password"]
					if !ok {
						return fmt.Errorf("Sensitive value not found for %s", prop.GetEndpointType())
					}
					nestedBlock.SetAttributeValue("password", cty.StringVal(value))
				case "logging_gcs":
					value, ok := prop.SensitiveValues["gcs_secret_key"]
					if !ok {
						return fmt.Errorf("Sensitive value not found for %s", prop.GetEndpointType())
					}
					nestedBlock.SetAttributeValue("secret_key", cty.StringVal(value))
				case "logging_googlepubsub":
					value, ok := prop.SensitiveValues["pubsub_secret_key"]
					if !ok {
						return fmt.Errorf("Sensitive value not found for %s", prop.GetEndpointType())
					}
					nestedBlock.SetAttributeValue("secret_key", cty.StringVal(value))
				case "logging_heroku":
					value, ok := prop.SensitiveValues["heroku_token"]
					if !ok {
						return fmt.Errorf("Sensitive value not found for %s", prop.GetEndpointType())
					}
					nestedBlock.SetAttributeValue("token", cty.StringVal(value))
				case "logging_honeycomb":
					value, ok := prop.SensitiveValues["honeycomb_token"]
					if !ok {
						return fmt.Errorf("Sensitive value not found for %s", prop.GetEndpointType())
					}
					nestedBlock.SetAttributeValue("token", cty.StringVal(value))
				case "logging_https":
					value, ok := prop.SensitiveValues["https_tls_client_key"]
					if !ok {
						return fmt.Errorf("Sensitive value not found for %s", prop.GetEndpointType())
					}
					nestedBlock.SetAttributeValue("tls_client_key", cty.StringVal(value))
				case "logging_kafka":
					value, ok := prop.SensitiveValues["kafka_password"]
					if !ok {
						return fmt.Errorf("Sensitive value not found for %s", prop.GetEndpointType())
					}
					nestedBlock.SetAttributeValue("password", cty.StringVal(value))
					value, ok = prop.SensitiveValues["kafka_tls_client_key"]
					if !ok {
						return fmt.Errorf("Sensitive value not found for %s", prop.GetEndpointType())
					}
					nestedBlock.SetAttributeValue("tls_client_key", cty.StringVal(value))
				case "logging_kinesis":
					value, ok := prop.SensitiveValues["kinesis_access_key"]
					if !ok {
						return fmt.Errorf("Sensitive value not found for %s", prop.GetEndpointType())
					}
					nestedBlock.SetAttributeValue("access_key", cty.StringVal(value))
					value, ok = prop.SensitiveValues["kinesis_secret_key"]
					if !ok {
						return fmt.Errorf("Sensitive value not found for %s", prop.GetEndpointType())
					}
					nestedBlock.SetAttributeValue("secret_key", cty.StringVal(value))
				case "logging_logentries":
				case "logging_loggly":
					value, ok := prop.SensitiveValues["loggly_token"]
					if !ok {
						return fmt.Errorf("Sensitive value not found for %s", prop.GetEndpointType())
					}
					nestedBlock.SetAttributeValue("token", cty.StringVal(value))
				case "logging_logshuttle":
					value, ok := prop.SensitiveValues["logshuttle_token"]
					if !ok {
						return fmt.Errorf("Sensitive value not found for %s", prop.GetEndpointType())
					}
					nestedBlock.SetAttributeValue("token", cty.StringVal(value))
				case "logging_newrelic":
					value, ok := prop.SensitiveValues["newrelic_token"]
					if !ok {
						return fmt.Errorf("Sensitive value not found for %s", prop.GetEndpointType())
					}
					nestedBlock.SetAttributeValue("token", cty.StringVal(value))
				case "logging_openstack":
					value, ok := prop.SensitiveValues["openstack_access_key"]
					if !ok {
						return fmt.Errorf("Sensitive value not found for %s", prop.GetEndpointType())
					}
					nestedBlock.SetAttributeValue("access_key", cty.StringVal(value))
				case "logging_papertrail":
				case "logging_s3":
					value, ok := prop.SensitiveValues["s3_access_key"]
					if !ok {
						return fmt.Errorf("Sensitive value not found for %s", prop.GetEndpointType())
					}
					nestedBlock.SetAttributeValue("s3_access_key", cty.StringVal(value))
					value, ok = prop.SensitiveValues["s3_secret_key"]
					nestedBlock.SetAttributeValue("s3_secret_key", cty.StringVal(value))
				case "logging_scalyr":
					value, ok := prop.SensitiveValues["scalyr_token"]
					if !ok {
						return fmt.Errorf("Sensitive value not found for %s", prop.GetEndpointType())
					}
					nestedBlock.SetAttributeValue("token", cty.StringVal(value))
				case "logging_sftp":
					value, ok := prop.SensitiveValues["sftp_password"]
					if !ok {
						return fmt.Errorf("Sensitive value not found for %s", prop.GetEndpointType())
					}
					nestedBlock.SetAttributeValue("password", cty.StringVal(value))
					value, ok = prop.SensitiveValues["sftp_secret_key"]
					if !ok {
						return fmt.Errorf("Sensitive value not found for %s", prop.GetEndpointType())
					}
					nestedBlock.SetAttributeValue("secret_key", cty.StringVal(value))
				case "logging_splunk":
					value, ok := prop.SensitiveValues["splunk_tls_client_key"]
					if !ok {
						return fmt.Errorf("Sensitive value not found for %s", prop.GetEndpointType())
					}
					nestedBlock.SetAttributeValue("tls_client_key", cty.StringVal(value))
					value, ok = prop.SensitiveValues["splunk_token"]
					if !ok {
						return fmt.Errorf("Sensitive value not found for %s", prop.GetEndpointType())
					}
					nestedBlock.SetAttributeValue("token", cty.StringVal(value))
				case "logging_sumologic":
				case "logging_syslog":
					value, ok := prop.SensitiveValues["syslog_tls_client_key"]
					if !ok {
						return fmt.Errorf("Sensitive value not found for %s", prop.GetEndpointType())
					}
					nestedBlock.SetAttributeValue("tls_client_key", cty.StringVal(value))
				}
			}
		}
	}
	return nil
}

func rewriteACLResource(block *hclwrite.Block, serviceProp *VCLServiceResourceProp) error {
	body := block.Body()
	// remove read-only attributes
	body.RemoveAttribute("id")

	// set service_id to represent the resource dependency
	ref := getServiceIDReference(serviceProp)
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

func rewriteDictionaryResource(block *hclwrite.Block, serviceProp *VCLServiceResourceProp) error {
	body := block.Body()
	// remove read-only attributes
	body.RemoveAttribute("id")

	// set service_id to represent the resource dependency
	ref := getServiceIDReference(serviceProp)
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

func rewriteDynamicSnippetResource(block *hclwrite.Block, serviceProp *VCLServiceResourceProp) error {
	body := block.Body()
	// remove read-only attributes
	body.RemoveAttribute("id")

	// set service_id to represent the resource dependency
	ref := getServiceIDReference(serviceProp)
	body.SetAttributeTraversal("service_id", ref)

	// replace content value with file()
	name := block.Labels()[1]
	path := fmt.Sprintf("./vcl/dsnippet_%s.vcl", normalizeName(name))
	tokens := getFileFunction(path)
	body.SetAttributeRaw("content", tokens)

	return nil
}

func getFileFunction(path string) hclwrite.Tokens {
	return hclwrite.Tokens{
		{Type: hclsyntax.TokenIdent, Bytes: []byte("file")},
		{Type: hclsyntax.TokenOParen, Bytes: []byte{'('}},
		{Type: hclsyntax.TokenOQuote, Bytes: []byte{'"'}},
		{Type: hclsyntax.TokenQuotedLit, Bytes: []byte(path)},
		{Type: hclsyntax.TokenCQuote, Bytes: []byte{'"'}},
		{Type: hclsyntax.TokenCParen, Bytes: []byte{')'}},
	}
}

func getServiceIDReference(serviceProp *VCLServiceResourceProp) hcl.Traversal {
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
		return "", fmt.Errorf(`failed to find "%s" in "%s"`, attrKey, block.Type())
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

func isJSONEncoded(block *hclwrite.Block, attrKey string) (bool, error) {
	attr := block.Body().GetAttribute(attrKey)
	if attr == nil {
		return false, fmt.Errorf(`failed to find "%s" in "%s"`, attrKey, block.Type())
	}

	expr := attr.Expr()
	exprTokens := expr.BuildTokens(nil)
	if exprTokens[0].Type == hclsyntax.TokenIdent && string(exprTokens[0].Bytes) == "jsonencode" {
		return true, nil
	}

	return false, nil
}
