package main

import (
	"errors"
	"flag"
	"fmt"
	"strings"

	"aether-dots/internal/store"
	"aether-dots/internal/task"
)

func skillsCommand(db *store.Store, args []string) error {
	if len(args) == 0 || args[0] == "list" {
		for _, skill := range db.Snapshot().Skills {
			fmt.Printf("%s\t%-28s\t%s\n", skill.ID, skill.Name, skill.Description)
		}
		return nil
	}
	switch args[0] {
	case "create":
		return writeSkill(db, "create", "", args[1:])
	case "update":
		if len(args) < 2 {
			return errors.New("usage: aether skills update <skill-id> [skill fields]")
		}
		return writeSkill(db, "update", args[1], args[2:])
	case "inspect":
		if len(args) != 2 {
			return errors.New("usage: aether skills inspect <skill-id>")
		}
		for _, skill := range db.Snapshot().Skills {
			if skill.ID == args[1] {
				printSkill(skill)
				return nil
			}
		}
		return errors.New("skill not found")
	case "delete":
		if len(args) != 2 {
			return errors.New("usage: aether skills delete <skill-id>")
		}
		return db.DeleteSkill(args[1])
	default:
		return errors.New("usage: aether skills [list|create|update|inspect|delete]")
	}
}

func writeSkill(db *store.Store, operation, id string, args []string) error {
	var existing task.Skill
	if operation == "update" {
		for _, skill := range db.Snapshot().Skills {
			if skill.ID == id {
				existing = skill
				break
			}
		}
		if existing.ID == "" {
			return errors.New("skill not found")
		}
	}
	fs := flag.NewFlagSet("skills "+operation, flag.ContinueOnError)
	name := fs.String("name", existing.Name, "skill name")
	description := fs.String("description", existing.Description, "short skill description")
	when := fs.String("when-to-use", existing.WhenToUse, "conditions for using the skill")
	inputs := fs.String("inputs", existing.Inputs, "required inputs and source context")
	steps := fs.String("steps", existing.Steps, "ordered workflow steps and decision rules")
	validation := fs.String("validation", existing.Validation, "how to check the result")
	output := fs.String("output", existing.Output, "expected response format")
	approval := fs.String("approval", existing.Approval, "actions or decisions that need approval")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	in := task.CreateSkill{
		Name: *name, Description: *description, WhenToUse: *when, Inputs: *inputs,
		Steps: *steps, Validation: *validation, Output: *output, Approval: *approval,
	}
	if operation == "update" {
		if err := db.UpdateSkill(id, in); err != nil {
			return err
		}
		fmt.Printf("Updated skill %q (%s)\n", *name, id)
		return nil
	}
	skill, err := db.CreateSkill(in)
	if err == nil {
		fmt.Printf("Created skill %q (%s)\n", skill.Name, skill.ID)
	}
	return err
}

func printSkill(skill task.Skill) {
	fmt.Printf("Skill %s — %s\nDescription: %s\nWhen to use: %s\nRequired inputs: %s\nSteps: %s\nValidation: %s\nExpected output: %s\nApproval boundary: %s\n",
		skill.ID, skill.Name, skill.Description, skill.WhenToUse, skill.Inputs,
		skill.Steps, skill.Validation, skill.Output, skill.Approval)
}
