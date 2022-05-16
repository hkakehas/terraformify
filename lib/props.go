package terraformify

import (
	"regexp"
	"strconv"
	"strings"
)

type TFBlockProp interface {
	GetType() string
	GetID() string
	GetIDforTFImport() string
	GetNormalizedName() string
	GetRef() string
}

type VCLServiceResourceProp struct {
	ID            string
	Name          string
	Version       int
	TargetVersion int
}

func NewVCLServiceResourceProp(id, name string, version, targetversion int) *VCLServiceResourceProp {
	return &VCLServiceResourceProp{
		ID:            id,
		Name:          name,
		Version:       version,
		TargetVersion: targetversion,
	}
}
func (v *VCLServiceResourceProp) GetType() string {
	return "fastly_service_vcl"
}
func (v *VCLServiceResourceProp) GetID() string {
	return v.ID
}
func (v *VCLServiceResourceProp) GetIDforTFImport() string {
	if v.TargetVersion != 0 {
		return v.GetID() + "@" + strconv.Itoa(v.GetVersion())
	}
	return v.GetID()
}
func (v *VCLServiceResourceProp) GetVersion() int {
	if v.TargetVersion != 0 {
		return v.TargetVersion
	}
	return v.Version
}
func (v *VCLServiceResourceProp) GetName() string {
	return v.Name
}
func (v *VCLServiceResourceProp) GetNormalizedName() string {
	// Check if the name can be used as a Terraform resource name
	// If not, falling back to the default resource name
	name := normalizeName(v.GetName())
	if !isValidResourceName(name) {
		name = "service"
	}
	return name
}
func (v *VCLServiceResourceProp) GetRef() string {
	return v.GetType() + "." + v.GetNormalizedName()
}

type WAFResourceProp struct {
	ID   string
	Name string
	*VCLServiceResourceProp
}

func NewWAFResourceProp(id string, sr *VCLServiceResourceProp) *WAFResourceProp {
	return &WAFResourceProp{
		ID:                     id,
		Name:                   "waf",
		VCLServiceResourceProp: sr,
	}
}
func (w *WAFResourceProp) GetType() string {
	return "fastly_service_waf_configuration"
}
func (w *WAFResourceProp) GetID() string {
	return w.ID
}
func (w *WAFResourceProp) GetIDforTFImport() string {
	return w.GetID()
}
func (w *WAFResourceProp) GetName() string {
	return w.Name
}
func (w *WAFResourceProp) GetNormalizedName() string {
	return normalizeName(w.GetName())
}
func (w *WAFResourceProp) GetRef() string {
	return w.GetType() + "." + w.GetNormalizedName()
}

type ACLResourceProp struct {
	ID   string
	Name string
	No   int
	*VCLServiceResourceProp
}

func NewACLResourceProp(id, name string, sr *VCLServiceResourceProp) *ACLResourceProp {
	return &ACLResourceProp{
		ID:                     id,
		Name:                   name,
		VCLServiceResourceProp: sr,
	}
}
func (a *ACLResourceProp) GetType() string {
	return "fastly_service_acl_entries"
}
func (a *ACLResourceProp) GetID() string {
	return a.ID
}
func (a *ACLResourceProp) GetIDforTFImport() string {
	return a.VCLServiceResourceProp.GetID() + "/" + a.ID
}
func (a *ACLResourceProp) GetName() string {
	return a.Name
}
func (a *ACLResourceProp) GetNormalizedName() string {
	return normalizeName(a.Name)
}
func (a *ACLResourceProp) GetRef() string {
	return a.GetType() + "." + a.GetNormalizedName()
}

type DictionaryResourceProp struct {
	ID   string
	Name string
	*VCLServiceResourceProp
}

func NewDictionaryResourceProp(id, name string, sr *VCLServiceResourceProp) *DictionaryResourceProp {
	return &DictionaryResourceProp{
		ID:                     id,
		Name:                   name,
		VCLServiceResourceProp: sr,
	}
}
func (d *DictionaryResourceProp) GetType() string {
	return "fastly_service_dictionary_items"
}
func (d *DictionaryResourceProp) GetID() string {
	return d.ID
}
func (d *DictionaryResourceProp) GetIDforTFImport() string {
	return d.VCLServiceResourceProp.GetID() + "/" + d.ID
}
func (d *DictionaryResourceProp) GetName() string {
	return d.Name
}
func (d *DictionaryResourceProp) GetNormalizedName() string {
	return normalizeName(d.GetName())
}
func (d *DictionaryResourceProp) GetRef() string {
	return d.GetType() + "." + d.GetNormalizedName()
}

type DynamicSnippetResourceProp struct {
	ID   string
	Name string
	*VCLServiceResourceProp
}

func NewDynamicSnippetResourceProp(id, name string, sr *VCLServiceResourceProp) *DynamicSnippetResourceProp {
	return &DynamicSnippetResourceProp{
		ID:                     id,
		Name:                   name,
		VCLServiceResourceProp: sr,
	}
}
func (ds *DynamicSnippetResourceProp) GetType() string {
	return "fastly_service_dynamic_snippet_content"
}
func (ds *DynamicSnippetResourceProp) GetID() string {
	return ds.ID
}
func (ds *DynamicSnippetResourceProp) GetIDforTFImport() string {
	return ds.VCLServiceResourceProp.GetID() + "/" + ds.ID
}
func (ds *DynamicSnippetResourceProp) GetName() string {
	return ds.Name
}
func (ds *DynamicSnippetResourceProp) GetNormalizedName() string {
	return normalizeName(ds.GetName())
}
func (ds *DynamicSnippetResourceProp) GetRef() string {
	return ds.GetType() + "." + ds.GetNormalizedName()
}

type SnippetBlockProp struct {
	Name string
	*VCLServiceResourceProp
}

func NewSnippetBlockProp(name string, sr *VCLServiceResourceProp) *SnippetBlockProp {
	return &SnippetBlockProp{
		Name:                   name,
		VCLServiceResourceProp: sr,
	}
}
func (s *SnippetBlockProp) GetType() string {
	return "snippet"
}
func (s *SnippetBlockProp) GetName() string {
	return s.Name
}
func (s *SnippetBlockProp) GetNormalizedName() string {
	return normalizeName(s.GetName())
}

type VCLBlockProp struct {
	Name string
	*VCLServiceResourceProp
}

func NewVCLBlockProp(name string, sr *VCLServiceResourceProp) *VCLBlockProp {
	return &VCLBlockProp{
		Name:                   name,
		VCLServiceResourceProp: sr,
	}
}
func (v *VCLBlockProp) GetType() string {
	return "vcl"
}
func (v *VCLBlockProp) GetName() string {
	return v.Name
}
func (v *VCLBlockProp) GetNormalizedName() string {
	return normalizeName(v.GetName())
}

type LoggingBlockProp struct {
	Name         string
	EndpointType string
	IsJSON       bool
	*VCLServiceResourceProp
}

func NewLoggingBlockProp(name, endpointType string, isJSON bool, sr *VCLServiceResourceProp) *LoggingBlockProp {
	return &LoggingBlockProp{
		EndpointType:           endpointType,
		Name:                   name,
		IsJSON:                 isJSON,
		VCLServiceResourceProp: sr,
	}
}
func (l *LoggingBlockProp) GetEndpointType() string {
	return l.EndpointType
}
func (l *LoggingBlockProp) GetName() string {
	return l.Name
}
func (l *LoggingBlockProp) GetNormalizedName() string {
	return normalizeName(l.GetName())
}

func normalizeName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, ".", "_")
	name = strings.ReplaceAll(name, "\n", "_")
	name = strings.ReplaceAll(name, "\t", "_")
	return strings.ReplaceAll(name, " ", "_")
}

func isValidResourceName(name string) bool {
	// Validate if the string can be used as a Terraform resource name
	// - No check is necessary for "fastly_service_waf_configuration" because the name is fixed to "waf"
	// - No check is necessary for the following resources because invalid names are not accepted at Fastly
	//	- "fastly_service_acl_entries"
	//	- "fastly_service_dictionary_items"
	//	- "fastly_service_dynamic_snippet_content"

	// A TF resource names begin with a letter or underscore and may contain only letters, digits, underscores, and dashes
	// Spaces and dots are allowed here since they are replaced with underscores in TFBlockProp.GetNormalizedName()
	return regexp.MustCompile(`^[A-Za-z_][0-9A-Za-z_.\-\\s]*$`).MatchString(name)
}
