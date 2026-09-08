package orgsnapshot

import (
	"fmt"
	"net/mail"
	"regexp"
	"strings"
)

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{2,50}$`)

// EntityType identifies the record type referenced by a ValidationError.
type EntityType string

const (
	EntityUser       EntityType = "user"
	EntityTeam       EntityType = "team"
	EntityMembership EntityType = "membership"
	EntitySnapshot   EntityType = "snapshot"
)

// ValidationError represents a single validation failure.
type ValidationError struct {
	Entity     EntityType `json:"entity"`
	Identifier string     `json:"identifier"`
	Field      string     `json:"field"`
	Message    string     `json:"message"`
}

// ValidationErrors is the complete set of contract validation failures.
type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	messages := make([]string, len(e))
	for i := range e {
		messages[i] = e[i].Error()
	}
	return strings.Join(messages, "; ")
}

func (e ValidationErrors) Unwrap() []error {
	errs := make([]error, len(e))
	for i := range e {
		errs[i] = e[i]
	}
	return errs
}

func (e ValidationError) Error() string {
	if e.Identifier != "" {
		return fmt.Sprintf("%s %q: %s: %s", e.Entity, e.Identifier, e.Field, e.Message)
	}
	return fmt.Sprintf("%s: %s: %s", e.Entity, e.Field, e.Message)
}

// Validate checks the snapshot against contract-level rules and returns
// all validation errors found.
func (s Snapshot) Validate() ValidationErrors {
	var errs ValidationErrors
	if s.ContractVersion != ContractVersion {
		errs = append(errs, ValidationError{
			Entity:  EntitySnapshot,
			Field:   "contractVersion",
			Message: fmt.Sprintf("unsupported contract version %q; expected %q", s.ContractVersion, ContractVersion),
		})
	}
	if s.GeneratedAt.IsZero() {
		errs = append(errs, ValidationError{
			Entity:  EntitySnapshot,
			Field:   "generatedAt",
			Message: "generatedAt is required",
		})
	}
	if len(s.Users) == 0 {
		errs = append(errs, ValidationError{
			Entity:  EntitySnapshot,
			Field:   "users",
			Message: "users must contain at least one record",
		})
	}
	if len(s.Teams) == 0 {
		errs = append(errs, ValidationError{
			Entity:  EntitySnapshot,
			Field:   "teams",
			Message: "teams must contain at least one record",
		})
	}

	userIDs := make(map[string]bool, len(s.Users))
	for i, u := range s.Users {
		identifier := u.ID
		if identifier == "" {
			identifier = fmt.Sprintf("index %d", i)
		} else if userIDs[u.ID] {
			identifier = fmt.Sprintf("index %d (id %s)", i, u.ID)
		}
		if u.ID == "" {
			errs = append(errs, ValidationError{
				Entity:     EntityUser,
				Identifier: identifier,
				Field:      "id",
				Message:    "id is required and must be non-empty",
			})
		} else if userIDs[u.ID] {
			errs = append(errs, ValidationError{
				Entity:     EntityUser,
				Identifier: identifier,
				Field:      "id",
				Message:    "duplicate id among users",
			})
		} else {
			userIDs[u.ID] = true
		}
		if !usernamePattern.MatchString(u.Username) {
			errs = append(errs, ValidationError{
				Entity:     EntityUser,
				Identifier: identifier,
				Field:      "username",
				Message:    "username must be 2-50 characters containing only letters, numbers, underscores, or hyphens",
			})
		}
		if u.DisplayName == "" {
			errs = append(errs, ValidationError{
				Entity:     EntityUser,
				Identifier: identifier,
				Field:      "displayName",
				Message:    "displayName is required and must be non-empty",
			})
		}
		if u.HierarchyLevelID == "" || len(u.HierarchyLevelID) > 50 {
			errs = append(errs, ValidationError{
				Entity:     EntityUser,
				Identifier: identifier,
				Field:      "hierarchyLevelId",
				Message:    "hierarchyLevelId is required and must not exceed 50 characters",
			})
		}
		if address, err := mail.ParseAddress(u.Email); err != nil || address.Address != u.Email {
			errs = append(errs, ValidationError{
				Entity:     EntityUser,
				Identifier: identifier,
				Field:      "email",
				Message:    "email must be a valid address",
			})
		}
	}

	teamIDs := make(map[string]bool, len(s.Teams))
	for i, t := range s.Teams {
		identifier := t.ID
		if identifier == "" {
			identifier = fmt.Sprintf("index %d", i)
		} else if teamIDs[t.ID] {
			identifier = fmt.Sprintf("index %d (id %s)", i, t.ID)
		}
		if t.ID == "" {
			errs = append(errs, ValidationError{
				Entity:     EntityTeam,
				Identifier: identifier,
				Field:      "id",
				Message:    "id is required and must be non-empty",
			})
		} else if teamIDs[t.ID] {
			errs = append(errs, ValidationError{
				Entity:     EntityTeam,
				Identifier: identifier,
				Field:      "id",
				Message:    "duplicate id among teams",
			})
		} else {
			teamIDs[t.ID] = true
		}
		if t.Name == "" {
			errs = append(errs, ValidationError{
				Entity:     EntityTeam,
				Identifier: t.ID,
				Field:      "name",
				Message:    "name is required and must be non-empty",
			})
		}
	}

	type membershipKey struct {
		userID string
		teamID string
	}
	membershipIDs := make(map[membershipKey]bool, len(s.Memberships))
	for _, m := range s.Memberships {
		identifier := fmt.Sprintf("userId=%q teamId=%q", m.UserID, m.TeamID)
		key := membershipKey{userID: m.UserID, teamID: m.TeamID}
		if membershipIDs[key] {
			errs = append(errs, ValidationError{
				Entity:     EntityMembership,
				Identifier: identifier,
				Field:      "userId,teamId",
				Message:    "duplicate membership",
			})
		} else {
			membershipIDs[key] = true
		}

		if !userIDs[m.UserID] {
			errs = append(errs, ValidationError{
				Entity:     EntityMembership,
				Identifier: identifier,
				Field:      "userId",
				Message: fmt.Sprintf(
					"references unknown user id %q",
					m.UserID,
				),
			})
		}

		if !teamIDs[m.TeamID] {
			errs = append(errs, ValidationError{
				Entity:     EntityMembership,
				Identifier: identifier,
				Field:      "teamId",
				Message: fmt.Sprintf(
					"references unknown team id %q",
					m.TeamID,
				),
			})
		}
	}

	for _, u := range s.Users {
		if u.ReportsToID == nil {
			continue
		}
		managerID := *u.ReportsToID
		if managerID == "" {
			errs = append(errs, ValidationError{
				Entity:     EntityUser,
				Identifier: u.ID,
				Field:      "reportsToId",
				Message:    "reportsToId must be null or a non-empty user id",
			})
			continue
		}

		if managerID == u.ID {
			errs = append(errs, ValidationError{
				Entity:     EntityUser,
				Identifier: u.ID,
				Field:      "reportsToId",
				Message:    "user cannot be their own manager",
			})
			continue
		}

		if !userIDs[managerID] {
			errs = append(errs, ValidationError{
				Entity:     EntityUser,
				Identifier: u.ID,
				Field:      "reportsToId",
				Message: fmt.Sprintf(
					"references unknown user id %q",
					managerID,
				),
			})
		}
	}

	for _, t := range s.Teams {
		if t.TeamLeadID == nil {
			continue
		}
		leadID := *t.TeamLeadID
		if leadID == "" || !userIDs[leadID] {
			errs = append(errs, ValidationError{
				Entity:     EntityTeam,
				Identifier: t.ID,
				Field:      "teamLeadId",
				Message:    fmt.Sprintf("references unknown user id %q", leadID),
			})
			continue
		}
		if !membershipIDs[membershipKey{userID: leadID, teamID: t.ID}] {
			errs = append(errs, ValidationError{
				Entity:     EntityTeam,
				Identifier: t.ID,
				Field:      "teamLeadId",
				Message:    fmt.Sprintf("team lead %q must be a member of the team", leadID),
			})
		}
	}

	reportsTo := make(map[string]string, len(s.Users))
	for _, u := range s.Users {
		if u.ID != "" && u.ReportsToID != nil && *u.ReportsToID != "" && *u.ReportsToID != u.ID && userIDs[*u.ReportsToID] {
			reportsTo[u.ID] = *u.ReportsToID
		}
	}
	const (
		visiting = 1
		visited  = 2
	)
	states := make(map[string]int, len(reportsTo))
	cycleUsers := make(map[string]bool)
	var visit func(string, []string, map[string]int)
	visit = func(userID string, path []string, positions map[string]int) {
		if userID == "" || states[userID] == visited {
			return
		}
		if position, ok := positions[userID]; ok {
			for _, cycleUserID := range path[position:] {
				cycleUsers[cycleUserID] = true
			}
			return
		}
		if states[userID] == visiting {
			return
		}
		states[userID] = visiting
		positions[userID] = len(path)
		visit(reportsTo[userID], append(path, userID), positions)
		delete(positions, userID)
		states[userID] = visited
	}
	for _, user := range s.Users {
		visit(user.ID, nil, make(map[string]int))
	}
	for _, user := range s.Users {
		if cycleUsers[user.ID] {
			errs = append(errs, ValidationError{
				Entity:     EntityUser,
				Identifier: user.ID,
				Field:      "reportsToId",
				Message:    "reporting hierarchy contains a cycle",
			})
		}
	}

	return errs
}

// ValidationError returns nil for a valid snapshot or all validation failures as one error.
func (s Snapshot) ValidationError() error {
	errs := s.Validate()
	if len(errs) == 0 {
		return nil
	}
	return errs
}

var _ error = ValidationErrors{}
