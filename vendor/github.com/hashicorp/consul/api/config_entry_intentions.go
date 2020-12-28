package api

import "time"

type ServiceIntentionsConfigEntry struct {
	Kind      string
	Name      string
	Namespace string `json:",omitempty"`

	Sources []*SourceIntention

	Meta map[string]string `json:",omitempty"`

	CreateIndex uint64
	ModifyIndex uint64
}

type SourceIntention struct {
	Name        string
	Namespace   string                 `json:",omitempty"`
	Action      IntentionAction        `json:",omitempty"`
	Permissions []*IntentionPermission `json:",omitempty"`
	Precedence  int
	Type        IntentionSourceType
	Description string `json:",omitempty"`

	LegacyID         string            `json:",omitempty" alias:"legacy_id"`
	LegacyMeta       map[string]string `json:",omitempty" alias:"legacy_meta"`
	LegacyCreateTime *time.Time        `json:",omitempty" alias:"legacy_create_time"`
	LegacyUpdateTime *time.Time        `json:",omitempty" alias:"legacy_update_time"`
}

func (e *ServiceIntentionsConfigEntry) GetKind() string {
	return e.Kind
}

func (e *ServiceIntentionsConfigEntry) GetName() string {
	return e.Name
}

func (e *ServiceIntentionsConfigEntry) GetNamespace() string {
	return e.Namespace
}

func (e *ServiceIntentionsConfigEntry) GetMeta() map[string]string {
	return e.Meta
}

func (e *ServiceIntentionsConfigEntry) GetCreateIndex() uint64 {
	return e.CreateIndex
}

func (e *ServiceIntentionsConfigEntry) GetModifyIndex() uint64 {
	return e.ModifyIndex
}

type IntentionPermission struct {
	Action IntentionAction
	HTTP   *IntentionHTTPPermission `json:",omitempty"`
}

type IntentionHTTPPermission struct {
	PathExact  string `json:",omitempty" alias:"path_exact"`
	PathPrefix string `json:",omitempty" alias:"path_prefix"`
	PathRegex  string `json:",omitempty" alias:"path_regex"`

	Header []IntentionHTTPHeaderPermission `json:",omitempty"`

	Methods []string `json:",omitempty"`
}

type IntentionHTTPHeaderPermission struct {
	Name    string
	Present bool   `json:",omitempty"`
	Exact   string `json:",omitempty"`
	Prefix  string `json:",omitempty"`
	Suffix  string `json:",omitempty"`
	Regex   string `json:",omitempty"`
	Invert  bool   `json:",omitempty"`
}
