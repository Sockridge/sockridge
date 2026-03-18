package socialagents

// AgentCard is the core model representing an agent in the registry.
type AgentCard struct {
	ID              string
	Name            string
	Description     string
	URL             string
	Version         string
	ProtocolVersion string
	Skills          []Skill
	Capabilities    *Capabilities
	PublisherID     string
	Status          string
	GatekeeperResult *GatekeeperResult
}

// Skill represents a capability an agent exposes.
type Skill struct {
	ID          string
	Name        string
	Description string
	Tags        []string
}

// Capabilities describes what communication features an agent supports.
type Capabilities struct {
	Streaming         bool
	PushNotifications bool
	MultiTurn         bool
	ToolUse           bool
}

// GatekeeperResult holds the result of the automated validation pipeline.
type GatekeeperResult struct {
	Approved        bool
	ConfidenceScore float32
	Reason          string
	Reachable       bool
	PingLatencyMs   int32
}

// ResolvedEndpoint is returned by ResolveEndpoint.
type ResolvedEndpoint struct {
	URL       string
	Transport string
	Agent     *AgentCard
}

// Agreement represents a mutual access contract between two publishers.
type Agreement struct {
	ID          string
	RequesterID string
	ReceiverID  string
	Message     string
	Status      string
	SharedKey   string
}

// SearchResult is returned by SemanticSearch.
type SearchResult struct {
	Agent *AgentCard
	Score float32
}
