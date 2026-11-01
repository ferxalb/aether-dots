package store

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"aether-dots/internal/task"
)

func (s *Store) CreateSkill(in task.CreateSkill) (task.Skill, error) {
	skill := normalizeSkill(task.Skill{
		Name: in.Name, Description: in.Description, WhenToUse: in.WhenToUse,
		Inputs: in.Inputs, Steps: in.Steps, Validation: in.Validation,
		Output: in.Output, Approval: in.Approval,
	})
	if err := validateSkill(skill); err != nil {
		return task.Skill{}, err
	}
	id, err := newID()
	if err != nil {
		return task.Skill{}, err
	}
	now := time.Now().UTC()
	skill.ID, skill.CreatedAt, skill.UpdatedAt = id, now, now
	err = s.change(func(db *task.Database) error {
		if skillNameExists(db.Skills, skill.Name, "") {
			return fmt.Errorf("skill %q already exists", skill.Name)
		}
		db.Skills = append(db.Skills, skill)
		return nil
	})
	return skill, err
}

func (s *Store) UpdateSkill(id string, in task.CreateSkill) error {
	skill := normalizeSkill(task.Skill{
		ID: id, Name: in.Name, Description: in.Description, WhenToUse: in.WhenToUse,
		Inputs: in.Inputs, Steps: in.Steps, Validation: in.Validation,
		Output: in.Output, Approval: in.Approval,
	})
	if err := validateSkill(skill); err != nil {
		return err
	}
	return s.change(func(db *task.Database) error {
		i := skillIndex(db, id)
		if i < 0 {
			return errors.New("skill not found")
		}
		if skillNameExists(db.Skills, skill.Name, id) {
			return fmt.Errorf("skill %q already exists", skill.Name)
		}
		skill.CreatedAt = db.Skills[i].CreatedAt
		skill.UpdatedAt = time.Now().UTC()
		db.Skills[i] = skill
		return nil
	})
}

func (s *Store) DeleteSkill(id string) error {
	return s.change(func(db *task.Database) error {
		i := skillIndex(db, id)
		if i < 0 {
			return errors.New("skill not found")
		}
		db.Skills = append(db.Skills[:i], db.Skills[i+1:]...)
		return nil
	})
}

func normalizeSkill(skill task.Skill) task.Skill {
	skill.Name = strings.TrimSpace(skill.Name)
	skill.Description = strings.TrimSpace(skill.Description)
	skill.WhenToUse = strings.TrimSpace(skill.WhenToUse)
	skill.Inputs = strings.TrimSpace(skill.Inputs)
	skill.Steps = strings.TrimSpace(skill.Steps)
	skill.Validation = strings.TrimSpace(skill.Validation)
	skill.Output = strings.TrimSpace(skill.Output)
	skill.Approval = strings.TrimSpace(skill.Approval)
	return skill
}

func validateSkill(skill task.Skill) error {
	if skill.Name == "" || len(skill.Name) > task.MaxSkillNameBytes {
		return fmt.Errorf("skill name must be 1–%d bytes", task.MaxSkillNameBytes)
	}
	if strings.ContainsAny(skill.Name, "[]\r\n") {
		return errors.New("skill name cannot contain brackets or line breaks because brackets select it in chat")
	}
	if len(skill.Description) > task.MaxSkillDescriptionBytes {
		return fmt.Errorf("skill description must be at most %d bytes", task.MaxSkillDescriptionBytes)
	}
	fields := []string{skill.Name, skill.Description, skill.WhenToUse, skill.Inputs, skill.Steps, skill.Validation, skill.Output, skill.Approval}
	for _, value := range fields {
		if !utf8.ValidString(value) {
			return errors.New("skill fields must be valid UTF-8 text")
		}
	}
	for _, value := range []string{skill.WhenToUse, skill.Inputs, skill.Steps, skill.Validation, skill.Output, skill.Approval} {
		if len(value) > task.MaxSkillFieldBytes {
			return fmt.Errorf("each skill instruction field must be at most %d bytes", task.MaxSkillFieldBytes)
		}
	}
	if skill.WhenToUse == "" || skill.Inputs == "" || skill.Steps == "" || skill.Validation == "" || skill.Output == "" || skill.Approval == "" {
		return errors.New("a skill needs when-to-use, inputs, steps, validation, output, and approval guidance")
	}
	definitionBytes := len(skill.Name) + len(skill.Description) + len(skill.WhenToUse) + len(skill.Inputs) + len(skill.Steps) + len(skill.Validation) + len(skill.Output) + len(skill.Approval)
	if definitionBytes > task.MaxSkillDefinitionBytes {
		return fmt.Errorf("skill definition must be at most %d bytes in total", task.MaxSkillDefinitionBytes)
	}
	return nil
}

func skillIndex(db *task.Database, id string) int {
	for i := range db.Skills {
		if db.Skills[i].ID == id {
			return i
		}
	}
	return -1
}

func skillNameExists(skills []task.Skill, name, exceptID string) bool {
	for _, skill := range skills {
		if skill.ID != exceptID && strings.EqualFold(skill.Name, name) {
			return true
		}
	}
	return false
}

func selectSkill(skills []task.Skill, message string) (task.Skill, bool) {
	lower := strings.ToLower(message)
	for _, skill := range skills {
		if strings.Contains(lower, strings.ToLower("@skill["+skill.Name+"]")) {
			return skill, true
		}
	}
	return task.Skill{}, false
}
