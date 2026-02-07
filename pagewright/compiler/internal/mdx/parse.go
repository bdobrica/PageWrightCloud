package mdx

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bogdan/pagewright/compiler/internal/types"
)

// Parse parses markdown with MDX component blocks into nodes
func Parse(source []byte) ([]types.MDXNode, error) {
	var nodes []types.MDXNode
	scanner := bufio.NewScanner(bytes.NewReader(source))

	var currentMarkdown strings.Builder
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Check for component block start
		if strings.HasPrefix(strings.TrimSpace(line), ":::component ") {
			// Save any accumulated markdown
			if currentMarkdown.Len() > 0 {
				nodes = append(nodes, types.MarkdownNode{
					Text: currentMarkdown.String(),
				})
				currentMarkdown.Reset()
			}

			// Parse component block
			componentNode, err := parseComponentBlock(scanner, line, lineNum)
			if err != nil {
				return nil, err
			}
			nodes = append(nodes, componentNode)
			continue
		}

		// Regular markdown line
		currentMarkdown.WriteString(line)
		currentMarkdown.WriteString("\n")
	}

	// Save any remaining markdown
	if currentMarkdown.Len() > 0 {
		nodes = append(nodes, types.MarkdownNode{
			Text: currentMarkdown.String(),
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return nodes, nil
}

// parseComponentBlock parses a single :::component block
func parseComponentBlock(scanner *bufio.Scanner, firstLine string, startLine int) (types.ComponentNode, error) {
	// Extract component name
	parts := strings.Fields(strings.TrimSpace(firstLine))
	if len(parts) < 2 {
		return types.ComponentNode{}, &types.CompileError{
			Line:    startLine,
			Message: "component block missing name: " + firstLine,
		}
	}

	componentName := parts[1]
	props := make(map[string]interface{})

	// Parse properties until we hit :::
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Check for end of block
		if trimmed == ":::" {
			return types.ComponentNode{
				Name:  componentName,
				Props: props,
				Line:  startLine,
			}, nil
		}

		// Skip empty lines
		if trimmed == "" {
			continue
		}

		// Parse key: value line
		colonIdx := strings.Index(line, ":")
		if colonIdx == -1 {
			return types.ComponentNode{}, &types.CompileError{
				Line:    startLine,
				Message: fmt.Sprintf("invalid property line (missing colon): %s", line),
			}
		}

		key := strings.TrimSpace(line[:colonIdx])
		valueStr := strings.TrimSpace(line[colonIdx+1:])

		// Parse value as JSON
		value, err := parseJSONValue(valueStr)
		if err != nil {
			return types.ComponentNode{}, &types.CompileError{
				Line:    startLine,
				Message: fmt.Sprintf("invalid JSON value for property '%s': %v", key, err),
			}
		}

		props[key] = value
	}

	return types.ComponentNode{}, &types.CompileError{
		Line:    startLine,
		Message: "component block not closed (missing :::)",
	}
}

// parseJSONValue parses a JSON value and validates that leaf values are strings
func parseJSONValue(valueStr string) (interface{}, error) {
	// Try to parse as JSON
	var value interface{}
	if err := json.Unmarshal([]byte(valueStr), &value); err != nil {
		// If it fails, treat it as a plain string (no quotes)
		return valueStr, nil
	}

	// Validate that all leaf values are strings
	if err := validateStringLeaves(value); err != nil {
		return nil, err
	}

	return value, nil
}

// validateStringLeaves ensures all leaf values in the structure are strings
func validateStringLeaves(value interface{}) error {
	switch v := value.(type) {
	case string:
		return nil
	case []interface{}:
		for i, item := range v {
			if err := validateStringLeaves(item); err != nil {
				return fmt.Errorf("array[%d]: %w", i, err)
			}
		}
		return nil
	case map[string]interface{}:
		for key, val := range v {
			if err := validateStringLeaves(val); err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
		}
		return nil
	case float64, bool:
		return fmt.Errorf("leaf values must be strings, got %T", v)
	case nil:
		return nil
	default:
		return fmt.Errorf("unsupported type %T", v)
	}
}
