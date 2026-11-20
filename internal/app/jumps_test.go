package app

import (
	"strings"
	"testing"

	"xu/internal/store"
)

func jumpTestProfiles() []store.Profile {
	return []store.Profile{
		{ID: 1, Name: "bastion", Host: "10.0.0.1", Port: 22, Username: "ops", AuthType: "key"},
		{ID: 2, Name: "inner", Host: "10.1.0.2", Port: 22, Username: "deploy", AuthType: "key", JumpProfileID: 1},
		{ID: 3, Name: "deep", Host: "10.2.0.3", Port: 2222, Username: "root", AuthType: "agent", JumpProfileID: 2},
	}
}

func TestSSHProfileExpandsJumpChain(t *testing.T) {
	a := New()
	a.profiles = jumpTestProfiles()

	sp, err := a.sshProfileForSaved(a.profiles[2])
	if err != nil {
		t.Fatal(err)
	}
	if sp.Host != "10.2.0.3" || sp.Jump == nil || sp.Jump.Host != "10.1.0.2" {
		t.Fatalf("target/jump = %+v / %+v", sp, sp.Jump)
	}
	if sp.Jump.Jump == nil || sp.Jump.Jump.Host != "10.0.0.1" {
		t.Fatalf("nested jump = %+v", sp.Jump.Jump)
	}
	if sp.Jump.Jump.AuthType != "key" || sp.AuthType != "agent" {
		t.Fatalf("auth types = %q / %q", sp.Jump.Jump.AuthType, sp.AuthType)
	}
}

func TestJumpCycleIsRejected(t *testing.T) {
	a := New()
	a.profiles = []store.Profile{
		{ID: 1, Name: "a", Host: "a", Port: 22, Username: "root", JumpProfileID: 2},
		{ID: 2, Name: "b", Host: "b", Port: 22, Username: "root", JumpProfileID: 1},
	}
	if _, err := a.sshProfileForSaved(a.profiles[0]); err == nil || !strings.Contains(err.Error(), "循环") {
		t.Fatalf("cycle error = %v", err)
	}
}

func TestJumpOptionsExcludeSelfAndDownstream(t *testing.T) {
	a := New()
	a.profiles = jumpTestProfiles()
	a.profileID = 1
	a.jumpChoice = jumpOptionLabel(a.profiles[2])
	a.ensureJumpChoice()
	if a.jumpChoice != directJumpChoice {
		t.Fatalf("invalid jump choice survived: %q", a.jumpChoice)
	}

	options := a.jumpOptions()
	if len(options) != 1 || options[0] != directJumpChoice {
		t.Fatalf("options for bastion = %q", options)
	}

	a.profileID = 2
	options = a.jumpOptions()
	if len(options) != 2 || !strings.Contains(options[1], "#1") {
		t.Fatalf("options for inner = %q", options)
	}
	for _, option := range options {
		if strings.Contains(option, "#2") || strings.Contains(option, "#3") {
			t.Fatalf("self/downstream option leaked: %q", options)
		}
	}
}

func TestJumpSelectionValidation(t *testing.T) {
	a := New()
	a.profiles = jumpTestProfiles()
	a.jumpChoice = jumpOptionLabel(a.profiles[1])
	id, err := a.validateJumpChoice()
	if err != nil {
		t.Fatal(err)
	}
	if id != 2 {
		t.Fatalf("jump id = %d, want 2", id)
	}

	a.profileID = 2
	a.jumpChoice = jumpOptionLabel(a.profiles[2])
	if _, err := a.validateJumpChoice(); err == nil {
		t.Fatal("a profile must not use its downstream as a jump")
	}
}

func TestEditProfileRestoresJumpChoice(t *testing.T) {
	a := New()
	a.profiles = jumpTestProfiles()
	a.editProfile(a.profiles[1])
	if a.jumpChoice != jumpOptionLabel(a.profiles[0]) {
		t.Fatalf("jump choice = %q, want %q", a.jumpChoice, jumpOptionLabel(a.profiles[0]))
	}
}
