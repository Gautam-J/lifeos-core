package service

// Source says how a record came into LifeOS.
type Source string

const (
	SourceManual    Source = "manual"    // Gautam, through a form, a Shortcut or a CLI command
	SourceImport    Source = "import"    // a CSV import
	SourceConnector Source = "connector" // a connector that pulls or receives data
	SourceAgent     Source = "agent"     // an LLM agent, through an MCP tool
)

// Provenance is recorded on every row that the service writes.
type Provenance struct {
	Source Source
}

func (p Provenance) validate() error {
	switch p.Source {
	case SourceManual, SourceImport, SourceConnector, SourceAgent:
		return nil
	}
	return invalid("source", "must be one of manual, import, connector, agent")
}
