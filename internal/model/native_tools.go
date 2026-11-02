package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
)

func nativeToolDefinitions() []openai.ChatCompletionToolUnionParam {
	return []openai.ChatCompletionToolUnionParam{
		openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        "aether_current_time",
			Description: param.NewOpt("Return the current local date and time. This function does not access the network or user data."),
			Parameters: shared.FunctionParameters{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			},
		}),
		openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        "aether_calculate",
			Description: param.NewOpt("Evaluate a basic arithmetic expression using numbers, parentheses, and +, -, *, /, and ^."),
			Parameters: shared.FunctionParameters{
				"type": "object",
				"properties": map[string]any{
					"expression": map[string]any{"type": "string", "description": "A basic arithmetic expression, up to 256 characters."},
				},
				"required":             []string{"expression"},
				"additionalProperties": false,
			},
		}),
	}
}

func executeNativeTool(name, rawArguments string) (string, error) {
	if len(rawArguments) > 1024 {
		return "", errors.New("function arguments exceed the 1 KiB limit")
	}
	switch name {
	case "aether_current_time":
		var args struct{}
		if err := decodeStrictJSON(rawArguments, &args); err != nil {
			return "", fmt.Errorf("invalid current-time arguments: %w", err)
		}
		return time.Now().Format(time.RFC3339), nil
	case "aether_calculate":
		var args struct {
			Expression string `json:"expression"`
		}
		if err := decodeStrictJSON(rawArguments, &args); err != nil {
			return "", fmt.Errorf("invalid calculator arguments: %w", err)
		}
		if strings.TrimSpace(args.Expression) == "" || len(args.Expression) > 256 {
			return "", errors.New("calculator expression must be between 1 and 256 characters")
		}
		value, err := evaluateExpression(args.Expression)
		if err != nil {
			return "", err
		}
		return strconv.FormatFloat(value, 'g', 12, 64), nil
	default:
		return "", errors.New("requested function is not allow-listed")
	}
}

func decodeStrictJSON(raw string, target any) error {
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

type arithmeticParser struct {
	input string
	index int
	steps int
	depth int
}

func evaluateExpression(input string) (float64, error) {
	p := arithmeticParser{input: input}
	value, err := p.parseSum()
	if err != nil {
		return 0, err
	}
	p.skipSpaces()
	if p.index != len(p.input) {
		return 0, errors.New("calculator expression contains an unsupported character")
	}
	if !finite(value) {
		return 0, errors.New("calculator result is outside the supported numeric range")
	}
	return value, nil
}

func (p *arithmeticParser) parseSum() (float64, error) {
	value, err := p.parseProduct()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpaces()
		if p.take('+') {
			right, err := p.parseProduct()
			if err != nil {
				return 0, err
			}
			value += right
		} else if p.take('-') {
			right, err := p.parseProduct()
			if err != nil {
				return 0, err
			}
			value -= right
		} else {
			return checked(value)
		}
		if !finite(value) {
			return 0, errors.New("calculator result is outside the supported numeric range")
		}
	}
}

func (p *arithmeticParser) parseProduct() (float64, error) {
	value, err := p.parsePower()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpaces()
		if p.take('*') {
			right, err := p.parsePower()
			if err != nil {
				return 0, err
			}
			value *= right
		} else if p.take('/') {
			right, err := p.parsePower()
			if err != nil {
				return 0, err
			}
			if right == 0 {
				return 0, errors.New("division by zero")
			}
			value /= right
		} else {
			return checked(value)
		}
		if !finite(value) {
			return 0, errors.New("calculator result is outside the supported numeric range")
		}
	}
}

func (p *arithmeticParser) parsePower() (float64, error) {
	value, err := p.parseUnary()
	if err != nil {
		return 0, err
	}
	p.skipSpaces()
	if p.take('^') {
		right, err := p.parsePower()
		if err != nil {
			return 0, err
		}
		value = math.Pow(value, right)
	}
	return checked(value)
}

func (p *arithmeticParser) parseUnary() (float64, error) {
	p.skipSpaces()
	if p.take('+') {
		return p.parseUnary()
	}
	if p.take('-') {
		value, err := p.parseUnary()
		if err != nil {
			return 0, err
		}
		return checked(-value)
	}
	return p.parsePrimary()
}

func (p *arithmeticParser) parsePrimary() (float64, error) {
	p.steps++
	if p.steps > 512 {
		return 0, errors.New("calculator expression is too complex")
	}
	p.skipSpaces()
	if p.take('(') {
		p.depth++
		if p.depth > 32 {
			return 0, errors.New("calculator expression is nested too deeply")
		}
		value, err := p.parseSum()
		p.skipSpaces()
		p.depth--
		if err != nil {
			return 0, err
		}
		if !p.take(')') {
			return 0, errors.New("calculator expression is missing a closing parenthesis")
		}
		return value, nil
	}
	start := p.index
	for p.index < len(p.input) {
		ch := p.input[p.index]
		if (ch >= '0' && ch <= '9') || ch == '.' || ch == 'e' || ch == 'E' || ((ch == '+' || ch == '-') && p.index > start && (p.input[p.index-1] == 'e' || p.input[p.index-1] == 'E')) {
			p.index++
			continue
		}
		break
	}
	if start == p.index {
		return 0, errors.New("calculator expression expected a number or parenthesis")
	}
	value, err := strconv.ParseFloat(p.input[start:p.index], 64)
	if err != nil || !finite(value) {
		return 0, errors.New("calculator expression contains an invalid number")
	}
	return value, nil
}

func (p *arithmeticParser) skipSpaces() {
	for p.index < len(p.input) {
		switch p.input[p.index] {
		case ' ', '\t', '\n', '\r':
			p.index++
		default:
			return
		}
	}
}

func (p *arithmeticParser) take(want byte) bool {
	if p.index < len(p.input) && p.input[p.index] == want {
		p.index++
		return true
	}
	return false
}

func checked(value float64) (float64, error) {
	if !finite(value) {
		return 0, errors.New("calculator result is outside the supported numeric range")
	}
	return value, nil
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
