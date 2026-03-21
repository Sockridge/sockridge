package gatekeeper

import (
	"testing"

	registryv1 "github.com/Sockridge/sockridge/server/gen/go/agentregistry/v1"
)

func TestValidate_ValidCard(t *testing.T) {
	agent := &registryv1.AgentCard{
		Name:        "FHIR Lab Agent",
		Description: "Analyzes lab trends from FHIR data",
		Skills: []*registryv1.Skill{
			{Id: "fhir.lab.analyze", Name: "Lab Analyzer"},
		},
	}

	if err := validate(agent); err != nil {
		t.Errorf("expected valid card, got error: %v", err)
	}
}

func TestValidate_MissingName(t *testing.T) {
	agent := &registryv1.AgentCard{
		Description: "Some description",
		Skills:      []*registryv1.Skill{{Id: "s.1", Name: "Skill"}},
	}
	if err := validate(agent); err == nil {
		t.Error("expected error for missing name")
	}
}

func TestValidate_ShortName(t *testing.T) {
	agent := &registryv1.AgentCard{
		Name:        "ab",
		Description: "Some description here",
		Skills:      []*registryv1.Skill{{Id: "s.1", Name: "Skill"}},
	}
	if err := validate(agent); err == nil {
		t.Error("expected error for name too short")
	}
}

func TestValidate_MissingDescription(t *testing.T) {
	agent := &registryv1.AgentCard{
		Name:   "Valid Name",
		Skills: []*registryv1.Skill{{Id: "s.1", Name: "Skill"}},
	}
	if err := validate(agent); err == nil {
		t.Error("expected error for missing description")
	}
}

func TestValidate_ShortDescription(t *testing.T) {
	agent := &registryv1.AgentCard{
		Name:        "Valid Name",
		Description: "Short",
		Skills:      []*registryv1.Skill{{Id: "s.1", Name: "Skill"}},
	}
	if err := validate(agent); err == nil {
		t.Error("expected error for description too short")
	}
}

func TestValidate_NoSkills(t *testing.T) {
	agent := &registryv1.AgentCard{
		Name:        "Valid Name",
		Description: "Valid description that is long enough",
	}
	if err := validate(agent); err == nil {
		t.Error("expected error for missing skills")
	}
}

func TestValidate_SkillMissingID(t *testing.T) {
	agent := &registryv1.AgentCard{
		Name:        "Valid Name",
		Description: "Valid description that is long enough",
		Skills:      []*registryv1.Skill{{Name: "No ID Skill"}},
	}
	if err := validate(agent); err == nil {
		t.Error("expected error for skill missing ID")
	}
}

func TestValidate_SkillMissingName(t *testing.T) {
	agent := &registryv1.AgentCard{
		Name:        "Valid Name",
		Description: "Valid description that is long enough",
		Skills:      []*registryv1.Skill{{Id: "skill.id"}},
	}
	if err := validate(agent); err == nil {
		t.Error("expected error for skill missing name")
	}
}

func TestValidate_MultipleSkills(t *testing.T) {
	agent := &registryv1.AgentCard{
		Name:        "Valid Name",
		Description: "Valid description that is long enough",
		Skills: []*registryv1.Skill{
			{Id: "s.1", Name: "Skill One"},
			{Id: "s.2", Name: "Skill Two"},
			{Id: "s.3", Name: "Skill Three"},
		},
	}
	if err := validate(agent); err != nil {
		t.Errorf("expected valid card with multiple skills, got: %v", err)
	}
}
