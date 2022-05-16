package terraformify

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

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
		t := block.Type()

		switch {
		case t == "acl":
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
		case t == "dictionary":
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
		case t == "waf":
			id, err := getStringAttributeValue(block, "waf_id")
			if err != nil {
				return nil, err
			}
			prop := NewWAFResourceProp(id, serviceProp)
			props = append(props, prop)
		case t == "dynamicsnippet":
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
		case t == "snippet":
			name, err := getStringAttributeValue(block, "name")
			if err != nil {
				return nil, err
			}
			prop := NewSnippetBlockProp(name, serviceProp)
			props = append(props, prop)
		case t == "vcl":
			name, err := getStringAttributeValue(block, "name")
			if err != nil {
				return nil, err
			}
			prop := NewVCLBlockProp(name, serviceProp)
			props = append(props, prop)
		case strings.HasPrefix(t, "logging_"):
			name, err := getStringAttributeValue(block, "name")
			if err != nil {
				return nil, err
			}
			isJSON, err := isJSONEncoded(block, "format")
			if err != nil {
				return nil, err
			}
			prop := NewLoggingBlockProp(name, t, isJSON, serviceProp)
			props = append(props, prop)
		}
	}
	return props, nil
}

func RewriteResources(rawHCL string, serviceProp *VCLServiceResourceProp) ([]byte, error) {
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
			rewriteVCLServiceResource(block, serviceProp)
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

func parseHCL(rawHCL string) (*hclwrite.File, error) {
	// "%" in log format conflicts with the HCL syntax.
	// To make hclwrite.ParseConfig() work, it must be escaped with an extra "%"
	rawHCL = strings.ReplaceAll(rawHCL, "%{", "%%{")

	f, diags := hclwrite.ParseConfig([]byte(rawHCL), "", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, fmt.Errorf("errors: %s", diags)
	}

	return f, nil
}

func rewriteVCLServiceResource(block *hclwrite.Block, serviceProp *VCLServiceResourceProp) error {
	body := block.Body()

	// Remove read-only attributes
	body.RemoveAttribute("id")
	body.RemoveAttribute("active_version")
	body.RemoveAttribute("cloned_version")

	for _, block := range body.Blocks() {
		t := block.Type()
		nb := block.Body()

		switch {
		case t == "acl":
			nb.RemoveAttribute("acl_id")
		case t == "dictionary":
			nb.RemoveAttribute("dictionary_id")
		case t == "waf":
			nb.RemoveAttribute("waf_id")
		case t == "dynamicsnippet":
			nb.RemoveAttribute("snippet_id")
		case t == "snippet":
			// replace content value with file()
			name, err := getStringAttributeValue(block, "name")
			if err != nil {
				return err
			}
			name = normalizeName(name)
			path := fmt.Sprintf("./vcl/snippet_%s.vcl", name)
			tokens := getFileFunction(path)
			nb.SetAttributeRaw("content", tokens)
		case t == "vcl":
			// replace content value with file()
			name, err := getStringAttributeValue(block, "name")
			if err != nil {
				return err
			}
			name = normalizeName(name)
			path := fmt.Sprintf("./vcl/%s.vcl", name)
			tokens := getFileFunction(path)
			nb.SetAttributeRaw("content", tokens)
		case strings.HasPrefix(t, "logging_"):
			// replace format value with file()
			name, err := getStringAttributeValue(block, "name")
			if err != nil {
				return err
			}
			isJSON, err := isJSONEncoded(block, "format")
			if err != nil {
				return err
			}
			name = normalizeName(name)
			ext := "txt"
			if isJSON {
				ext = "json"
			}
			path := fmt.Sprintf("./logformat/%s.%s", name, ext)
			tokens := getFileFunction(path)
			nb.SetAttributeRaw("format", tokens)
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
