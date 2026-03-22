package context

import (
	"encoding/json"
	"strings"
)

// Instruction represents a single instruction for the agent.
type Instruction struct {
	Value string
}

// Instructions is a list of instructions for the agent.
type Instructions []Instruction

// String returns the instructions as a string.
func (i Instructions) String() string {
	values := make([]string, 0, len(i))
	for _, instruction := range i {
		values = append(values, instruction.Value)
	}
	return strings.Join(values, "\n")
}

// MarshalJSON returns the instructions as a JSON string.
func (i Instructions) MarshalJSON() ([]byte, error) {
	return json.Marshal(i.String())
}

// UnmarshalJSON unmarshals the instructions from a JSON string.
func (i *Instructions) UnmarshalJSON(data []byte) error {
	var values []string
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	*i = make(Instructions, len(values))
	for idx, value := range values {
		(*i)[idx] = Instruction{Value: value}
	}
	return nil
}
