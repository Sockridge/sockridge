package sockridge

// Provider — who built and operates the agent
type Provider struct {
	Name    string
	URL     string
	Contact string
}

// Authentication — how callers authenticate with this agent
type Authentication struct {
	Scheme      string // none | api_key | oauth2 | openid
	Description string
	TokenURL    string
	Scopes      string
}

// SkillExample — example input/output pair for a skill
type SkillExample struct {
	Input  string
	Output string
}

// Skill represents a capability an agent exposes
type Skill struct {
	ID          string
	Name        string
	Description string
	Tags        []string
	InputModes  []string
	OutputModes []string
	Examples    []SkillExample
}

// Capabilities describes communication features
type Capabilities struct {
	Streaming         bool
	PushNotifications bool
	MultiTurn         bool
	ToolUse           bool
}

// GatekeeperResult holds the result of automated validation
type GatekeeperResult struct {
	Approved        bool
	ConfidenceScore float32
	Reason          string
	Reachable       bool
	PingLatencyMs   int32
	A2ACompliant    bool
	A2AMatchesCard  bool
}

// AgentCard is the core model — fully A2A spec compliant
type AgentCard struct {
	// required
	Name        string
	Description string
	URL         string

	// base fields
	ID              string
	Version         string
	ProtocolVersion string
	Skills          []Skill
	Capabilities    *Capabilities

	// A2A spec fields
	Provider          *Provider
	Authentication    *Authentication
	IconURL           string
	DocumentationURL  string
	ProtocolVersions  []string
	Extensions        []string

	// set by registry
	PublisherID      string
	Status           string
	GatekeeperResult *GatekeeperResult
}

// ResolvedEndpoint is returned by ResolveEndpoint
type ResolvedEndpoint struct {
	URL       string
	Transport string
	Agent     *AgentCard
}

// Agreement represents a mutual access contract
type Agreement struct {
	ID          string
	RequesterID string
	ReceiverID  string
	Message     string
	Status      string
	SharedKey   string
}

// SearchResult is returned by SemanticSearch
type SearchResult struct {
	Agent *AgentCard
	Score float32
}