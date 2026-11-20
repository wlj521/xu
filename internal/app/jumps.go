package app

import (
	"errors"
	"fmt"
	"strings"

	"xu/internal/sshx"
	"xu/internal/store"
)

const directJumpChoice = "直连（不使用跳板机）"

func (a *App) jumpOptions() []string {
	options := []string{directJumpChoice}
	for _, p := range a.profiles {
		if !a.canUseAsJump(p.ID) {
			continue
		}
		options = append(options, jumpOptionLabel(p))
	}
	return options
}

func jumpOptionLabel(p store.Profile) string {
	name := strings.TrimSpace(p.Name)
	if name == "" {
		name = profileDisplayName(p)
	}
	return fmt.Sprintf("#%d  %s · %s@%s:%d", p.ID, name, p.Username, p.Host, p.Port)
}

func (a *App) jumpIDByChoice(choice string) int64 {
	if choice == "" || choice == directJumpChoice {
		return 0
	}
	for _, p := range a.profiles {
		if jumpOptionLabel(p) == choice {
			return p.ID
		}
	}
	return 0
}

func (a *App) jumpChoiceForID(id int64) string {
	if id == 0 {
		return directJumpChoice
	}
	if p, ok := a.profileByID(id); ok {
		return jumpOptionLabel(p)
	}
	return directJumpChoice
}

func (a *App) canUseAsJump(id int64) bool {
	if id == 0 {
		return false
	}
	if a.profileID != 0 {
		if id == a.profileID || a.jumpChainContains(id, a.profileID) {
			return false
		}
	}
	return a.validateJumpChain(id, make(map[int64]bool)) == nil
}

// jumpChainContains reports whether profile startID already reaches needleID.
// It stops at malformed cycles instead of looping.
func (a *App) jumpChainContains(startID, needleID int64) bool {
	seen := make(map[int64]bool)
	for startID != 0 {
		if startID == needleID {
			return true
		}
		if seen[startID] {
			return false
		}
		seen[startID] = true
		p, ok := a.profileByID(startID)
		if !ok {
			return false
		}
		startID = p.JumpProfileID
	}
	return false
}

func (a *App) validateJumpChain(id int64, seen map[int64]bool) error {
	for id != 0 {
		if seen[id] {
			return errors.New("跳板机链路存在循环")
		}
		seen[id] = true
		p, ok := a.profileByID(id)
		if !ok {
			return fmt.Errorf("跳板机 #%d 不存在", id)
		}
		id = p.JumpProfileID
	}
	return nil
}

func (a *App) validateJumpChoice() (int64, error) {
	choice := strings.TrimSpace(a.jumpChoice)
	if choice == "" || choice == directJumpChoice {
		return 0, nil
	}
	id := a.jumpIDByChoice(choice)
	if id == 0 {
		return 0, errors.New("所选跳板机不存在，请重新选择")
	}
	if a.profileID != 0 {
		if id == a.profileID || a.jumpChainContains(id, a.profileID) {
			return 0, errors.New("不能选择当前连接自身或其下游作为跳板机")
		}
	}
	if err := a.validateJumpChain(id, make(map[int64]bool)); err != nil {
		return 0, err
	}
	return id, nil
}

func (a *App) ensureJumpChoice() {
	if a.jumpChoice == "" {
		a.jumpChoice = directJumpChoice
	}
	if a.jumpChoice == directJumpChoice {
		return
	}
	for _, option := range a.jumpOptions() {
		if option == a.jumpChoice {
			return
		}
	}
	a.jumpChoice = directJumpChoice
}

func (a *App) profileByID(id int64) (store.Profile, bool) {
	for _, p := range a.profiles {
		if p.ID == id {
			return p, true
		}
	}
	return store.Profile{}, false
}

func (a *App) jumpDisplayName(id int64) string {
	p, ok := a.profileByID(id)
	if !ok {
		return "直连"
	}
	return profileDisplayName(p)
}

// formJumpProfile resolves the jump selected in the connection form to the
// nested SSH profile used by the dialer.
func (a *App) formJumpProfile() (*sshx.Profile, error) {
	id, err := a.validateJumpChoice()
	if err != nil || id == 0 {
		return nil, err
	}
	p, ok := a.profileByID(id)
	if !ok {
		return nil, errors.New("所选跳板机不存在")
	}
	jump, err := a.sshProfileForSaved(p)
	if err != nil {
		return nil, err
	}
	return &jump, nil
}

// sshProfileForSaved expands a saved profile and its jump chain into the
// nested structure the SSH dialer uses.
func (a *App) sshProfileForSaved(p store.Profile) (sshx.Profile, error) {
	return a.sshProfileForSavedSeen(p, make(map[int64]bool))
}

func (a *App) sshProfileForSavedSeen(p store.Profile, seen map[int64]bool) (sshx.Profile, error) {
	if p.ID != 0 {
		if seen[p.ID] {
			return sshx.Profile{}, errors.New("跳板机链路存在循环")
		}
		seen[p.ID] = true
	}

	authType := strings.ToLower(strings.TrimSpace(p.AuthType))
	password := ""
	if isPasswordAuth(authType) {
		password = a.passwordForProfileID(p.ID)
	}
	out := sshx.Profile{
		Host:     p.Host,
		Port:     p.Port,
		Username: p.Username,
		Password: password,
		AuthType: authType,
	}

	if p.JumpProfileID != 0 {
		jump, ok := a.profileByID(p.JumpProfileID)
		if !ok {
			return sshx.Profile{}, fmt.Errorf("跳板机 #%d 不存在", p.JumpProfileID)
		}
		nested, err := a.sshProfileForSavedSeen(jump, seen)
		if err != nil {
			return sshx.Profile{}, err
		}
		out.Jump = &nested
	}
	return out, nil
}
