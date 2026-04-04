package instructions

import (
	"encoding/json"
	"strings"
)

type Instruction struct {
	Name  string
	Value string
}

type Instructions []Instruction

func (i Instructions) First() Instruction {
	if len(i) == 0 {
		return Instruction{}
	}

	return i[0]
}

func New(values ...string) Instructions {
	instructions := make(Instructions, 0, len(values))
	for _, value := range values {
		instructions = instructions.AppendValue(value)
	}

	return instructions
}

func (i Instructions) Append(instructions ...Instruction) Instructions {
	appended := make(Instructions, 0, len(i)+len(instructions))
	appended = append(appended, i...)
	for _, instruction := range instructions {
		if strings.TrimSpace(instruction.Value) == "" {
			continue
		}
		appended = append(appended, Instruction{
			Name:  strings.TrimSpace(instruction.Name),
			Value: strings.TrimSpace(instruction.Value),
		})
	}

	return appended
}

func (i Instructions) AppendValue(values ...string) Instructions {
	instructions := make([]Instruction, 0, len(values))
	for _, value := range values {
		instructions = append(instructions, Instruction{Value: value})
	}

	return i.Append(instructions...)
}

func (i Instructions) String() string {
	values := make([]string, 0, len(i))
	for _, instruction := range i {
		values = append(values, instruction.Value)
	}

	return strings.Join(values, "\n")
}

func (i Instructions) MarshalJSON() ([]byte, error) {
	return json.Marshal(i.String())
}

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
