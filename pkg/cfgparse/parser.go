package cfgparse

import (
	"errors"
	"fmt"
)

// NodeType represents the type of a structured config value.
type NodeType int

const (
	// NodeString represents a string config value.
	NodeString NodeType = iota
	// NodeNumber represents a numeric config value.
	NodeNumber
	// NodeBool represents a boolean config value.
	NodeBool
	// NodeNull represents a null config value.
	NodeNull
	// NodeObject represents an object config value.
	NodeObject
	// NodeArray represents an array config value.
	NodeArray
)

// ConfigEntry represents a flat key-value pair (properties, INI).
type ConfigEntry struct {
	Section string
	Key     string
	Value   string
	Index   int
	Comment string
}

// ConfigNode represents a node in a structured config tree (JSON, YAML, TOML, XML).
type ConfigNode struct {
	Key      string
	Value    string
	Type     NodeType
	Children []*ConfigNode
	Comment  string
}

// FlatConfigParser handles flat key-value formats.
type FlatConfigParser interface {
	Parse(data []byte) ([]ConfigEntry, error)
	Write(entries []ConfigEntry) ([]byte, error)
	Format() string
}

// StructuredConfigParser handles hierarchical formats.
type StructuredConfigParser interface {
	Parse(data []byte) (*ConfigNode, error)
	Write(root *ConfigNode) ([]byte, error)
	Format() string
}

// Parser is a union type returned by the registry.
type Parser struct {
	Flat       FlatConfigParser
	Structured StructuredConfigParser
}

// IsFlat returns true if this is a flat format parser.
func (p *Parser) IsFlat() bool {
	return p.Flat != nil
}

// ErrUnknownFormat is returned when a format string is not registered.
var ErrUnknownFormat = errors.New("unknown config format")

var (
	flatParsers       = map[string]FlatConfigParser{}
	structuredParsers = map[string]StructuredConfigParser{}
)

// RegisterFlat registers a flat format parser.
func RegisterFlat(p FlatConfigParser) {
	flatParsers[p.Format()] = p
}

// RegisterStructured registers a structured format parser.
func RegisterStructured(p StructuredConfigParser) {
	structuredParsers[p.Format()] = p
}

// GetParser returns the parser for the given format string.
func GetParser(format string) (*Parser, error) {
	if p, ok := flatParsers[format]; ok {
		return &Parser{Flat: p}, nil
	}
	if p, ok := structuredParsers[format]; ok {
		return &Parser{Structured: p}, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrUnknownFormat, format)
}
