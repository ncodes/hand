package context

import (
	"encoding/json"
	"strings"
)

// Instruction represents a single instruction for the agent.
type Instruction struct {
	Name  string
	Value string
}

// Instructions is a list of instructions for the agent.
type Instructions []Instruction

// First returns the first instruction in the list.
func (i Instructions) First() Instruction {
	if len(i) == 0 {
		return Instruction{}
	}
	return i[0]
}

// NewInstructions creates a new Instructions list from a variadic list of values.
func NewInstructions(values ...string) Instructions {
	instructions := make(Instructions, 0, len(values))
	for _, value := range values {
		instructions = instructions.ChainValue(value)
	}
	return instructions
}

func (i Instructions) Chain(instructions ...Instruction) Instructions {
	chained := make(Instructions, 0, len(i)+len(instructions))
	chained = append(chained, i...)

	for _, instruction := range instructions {
		if strings.TrimSpace(instruction.Value) == "" {
			continue
		}
		chained = append(chained, Instruction{
			Name:  strings.TrimSpace(instruction.Name),
			Value: strings.TrimSpace(instruction.Value),
		})
	}

	return chained
}

func (i Instructions) ChainValue(values ...string) Instructions {
	instructions := make([]Instruction, 0, len(values))
	for _, value := range values {
		instructions = append(instructions, Instruction{Value: value})
	}
	return i.Chain(instructions...)
}

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

func (i Instructions) GetByName(name string) (Instruction, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Instruction{}, false
	}
	for _, instruction := range i {
		if instruction.Name == name {
			return instruction, true
		}
	}
	return Instruction{}, false
}

func (i Instructions) WithoutName(name string) Instructions {
	name = strings.TrimSpace(name)
	if name == "" {
		return i
	}
	filtered := make(Instructions, 0, len(i))
	for _, instruction := range i {
		if instruction.Name == name {
			continue
		}
		filtered = append(filtered, instruction)
	}
	return filtered
}
